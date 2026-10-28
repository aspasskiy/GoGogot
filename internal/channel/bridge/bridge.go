package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/channel/render"
	"github.com/aspasskiy/gogogot/internal/transport"

	"github.com/rs/zerolog/log"
)

const (
	requestTimeout  = 30 * time.Second
	fileTimeout     = 3 * time.Minute
	minBackoff      = time.Second
	maxBackoff      = time.Minute
	stableAfter     = time.Minute // a run this long resets the backoff
	rollbackAfter   = 3           // quick crashes in a row before a bundled skill is rolled back
	stopGrace       = 3 * time.Second
	logLines        = 100
	maxInboundFile  = 50 << 20
	maxProtocolLine = 64 << 20
)

// State is where a channel is in its lifecycle.
type State string

const (
	StateDisabled   State = "disabled"    // channel.json has "enabled": false
	StateMissingEnv State = "missing_env" // required env vars are not set
	StateInvalid    State = "invalid"     // channel.json is broken
	StateReserved   State = "reserved"    // a built-in channel with this name is running
	StateStarting   State = "starting"
	StateRunning    State = "running"
	StateCrashed    State = "crashed" // exited; restarting after a backoff
	StateStopped    State = "stopped"
)

// Runnable reports whether the channel is running or trying to: false for
// the idle states (disabled, missing env, invalid, reserved).
func (s Status) Runnable() bool {
	switch s.State {
	case StateStarting, StateRunning, StateCrashed:
		return true
	}
	return false
}

// Status is a snapshot of one channel for the channel tools.
type Status struct {
	Name     string
	State    State
	Detail   string // why it is not running, or the last error
	PID      int
	Restarts int
	Since    time.Time
}

// bridge is one running channel skill. It is a render.Surface: every Surface
// call becomes a request to the child process.
type bridge struct {
	name  string
	dir   string
	man   Manifest
	inbox string

	handler  func() channel.Handler
	rollback func(name string) ([]string, error)
	// surf is what repliers hold: it resolves the channel by name on every
	// call, so a reply started before a restart goes out through the new
	// process.
	surf render.Surface
	work chan render.Inbound // inbound messages, dispatched in order off the read loop

	mu        sync.Mutex
	state     State
	detail    string
	pid       int
	restarts  int
	since     time.Time
	stdin     io.WriteCloser
	nextID    int
	pending   map[int]chan response
	ownerChat string
	lastChat  string
	logs      []string
	ready     chan struct{} // closed when the current process reports ready

	writeMu sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
}

type response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	MsgID textID `json:"msg_id"`
}

// textID is an ID the bridge may send as a JSON string or number (bridges
// written against APIs with numeric IDs often pass them through as is).
type textID string

func (t *textID) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*t = textID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("want a string or number ID, got %s", data)
	}
	*t = textID(n.String())
	return nil
}

// inbound is any line the bridge writes: an event or a response to a request.
type inbound struct {
	ID    int    `json:"id"`
	Event string `json:"event"`
	response

	OwnerChat textID `json:"owner_chat"`

	Chat   textID `json:"chat"`
	Sender textID `json:"sender"`
	Text   string `json:"text"`
	Files  []struct {
		Path string `json:"path"`
		Name string `json:"name"`
		Mime string `json:"mime"`
	} `json:"files"`
}

func newBridge(name, dir string, man Manifest, handler func() channel.Handler, surf render.Surface) *bridge {
	return &bridge{
		name:    name,
		dir:     dir,
		man:     man,
		inbox:   filepath.Join(os.TempDir(), "gogogot-channels", name),
		handler: handler,
		surf:    surf,
		work:    make(chan render.Inbound, 64),
		state:   StateStarting,
		pending: map[int]chan response{},
		ready:   make(chan struct{}),
	}
}

// start launches the supervisor: the process is restarted with backoff until
// stop is called. ctx is the manager's: runs dispatched from this bridge live
// on it, so stopping the bridge (a restart) does not cancel them.
func (b *bridge) start(ctx context.Context) {
	runCtx := ctx
	ctx, b.cancel = context.WithCancel(ctx)
	b.done = make(chan struct{})
	go b.dispatchLoop(ctx, runCtx)
	go func() {
		defer close(b.done)
		b.supervise(ctx)
	}()
}

func (b *bridge) stop() {
	if b.cancel == nil {
		return
	}
	b.cancel()
	<-b.done
	b.setState(StateStopped, "")
}

func (b *bridge) supervise(ctx context.Context) {
	backoff := minBackoff
	quickCrashes := 0
	rolledBack := false
	for {
		started := time.Now()
		err := b.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > stableAfter {
			backoff = minBackoff
			quickCrashes = 0
		} else {
			quickCrashes++
		}
		msg := "exited"
		if err != nil {
			msg = err.Error()
		}
		b.mu.Lock()
		b.restarts++
		b.mu.Unlock()

		if quickCrashes >= rollbackAfter && !rolledBack && b.tryRollback(msg) {
			rolledBack = true
			quickCrashes = 0
			backoff = minBackoff
			continue
		}

		b.setState(StateCrashed, fmt.Sprintf("%s; restarting in %s", msg, backoff))
		log.Warn().Str("channel", b.name).Str("reason", msg).Dur("backoff", backoff).Msg("channel bridge stopped, restarting")

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

// tryRollback restores the stock files of a bundled channel skill after it
// crashed rollbackAfter times in a row, keeping the broken files aside. It
// reports whether anything was restored.
func (b *bridge) tryRollback(reason string) bool {
	if b.rollback == nil {
		return false
	}
	restored, err := b.rollback(b.name)
	if err != nil {
		log.Error().Err(err).Str("channel", b.name).Msg("channel bridge: rollback failed")
		return false
	}
	if len(restored) == 0 {
		return false
	}
	if man, err := LoadManifest(b.dir); err == nil {
		b.mu.Lock()
		b.man = man
		b.mu.Unlock()
	}
	note := fmt.Sprintf("kept crashing (%s); restored stock %s, the broken versions are in .broken/",
		reason, strings.Join(restored, ", "))
	log.Warn().Str("channel", b.name).Msg("channel bridge " + note)
	b.mu.Lock()
	b.logs = append(b.logs, "[gogogot] "+note)
	b.mu.Unlock()
	return true
}

func (b *bridge) runOnce(ctx context.Context) error {
	b.mu.Lock()
	man := b.man
	b.mu.Unlock()

	if err := os.MkdirAll(b.inbox, 0o700); err != nil {
		return fmt.Errorf("create inbox: %w", err)
	}

	cmd := exec.CommandContext(ctx, man.Command[0], man.Command[1:]...)
	cmd.Dir = b.dir
	cmd.Env = append(os.Environ(),
		"GOGOGOT_CHANNEL="+b.name,
		"GOGOGOT_CHANNEL_INBOX="+b.inbox,
	)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = stopGrace

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	b.setState(StateStarting, "")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", strings.Join(man.Command, " "), err)
	}

	b.mu.Lock()
	b.stdin = stdin
	b.pid = cmd.Process.Pid
	select {
	case <-b.ready:
		// Ready was reported by the previous process; wait for this one.
		b.ready = make(chan struct{})
	default:
	}
	b.mu.Unlock()
	log.Info().Str("channel", b.name).Int("pid", cmd.Process.Pid).Msg("channel bridge started")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); b.readStderr(stderr) }()
	go func() { defer wg.Done(); b.readStdout(stdout) }()
	wg.Wait()
	err = cmd.Wait()

	b.mu.Lock()
	b.stdin = nil
	b.pid = 0
	pending := b.pending
	b.pending = map[int]chan response{}
	b.mu.Unlock()
	for _, ch := range pending {
		ch <- response{Error: "channel bridge exited"}
	}
	return err
}

func (b *bridge) readStderr(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		log.Info().Str("channel", b.name).Msg(line)
		b.mu.Lock()
		b.logs = append(b.logs, line)
		if len(b.logs) > logLines {
			b.logs = b.logs[len(b.logs)-logLines:]
		}
		b.mu.Unlock()
	}
}

func (b *bridge) readStdout(r io.Reader) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := readLine(br)
		if len(line) > 0 {
			b.handleLine(line)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				log.Warn().Err(err).Str("channel", b.name).Msg("channel bridge: read failed")
			}
			return
		}
	}
}

func readLine(br *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		line = append(line, chunk...)
		if len(line) > maxProtocolLine {
			return nil, errors.New("protocol line too long")
		}
		if err != nil || !isPrefix {
			return line, err
		}
	}
}

func (b *bridge) handleLine(line []byte) {
	var in inbound
	if err := json.Unmarshal(line, &in); err != nil {
		msg := "channel bridge: bad protocol line: " + err.Error()
		if !json.Valid(line) {
			msg = "channel bridge: not a JSON line (use stderr for logs)"
		}
		log.Warn().Str("channel", b.name).Str("line", truncate(string(line), 200)).Msg(msg)
		b.mu.Lock()
		b.logs = append(b.logs, "[gogogot] "+msg+": "+truncate(string(line), 200))
		b.mu.Unlock()
		return
	}

	switch in.Event {
	case "":
		b.mu.Lock()
		ch, ok := b.pending[in.ID]
		delete(b.pending, in.ID)
		b.mu.Unlock()
		if ok {
			ch <- in.response
		}

	case "ready":
		b.mu.Lock()
		if in.OwnerChat != "" {
			b.ownerChat = string(in.OwnerChat)
		}
		select {
		case <-b.ready:
		default:
			close(b.ready)
		}
		b.mu.Unlock()
		b.setState(StateRunning, "")

	case "message":
		b.acceptMessage(in)

	default:
		log.Warn().Str("channel", b.name).Str("event", in.Event).Msg("channel bridge: unknown event")
	}
}

func (b *bridge) acceptMessage(in inbound) {
	defer func() {
		for _, f := range in.Files {
			if b.inInbox(f.Path) {
				_ = os.Remove(f.Path)
			}
		}
	}()

	// The owner check lives here, not in the bridge: whatever the bridge
	// code says, only the owner reaches the agent.
	b.mu.Lock()
	man := b.man
	b.mu.Unlock()
	if owner := man.owner(); owner == "" || string(in.Sender) != owner {
		log.Warn().Str("channel", b.name).Str("sender", string(in.Sender)).Msg("channel bridge: dropped message from non-owner")
		return
	}
	if in.Chat == "" {
		log.Warn().Str("channel", b.name).Msg("channel bridge: message without chat")
		return
	}

	var atts []transport.Attachment
	for _, f := range in.Files {
		if !b.inInbox(f.Path) {
			log.Warn().Str("channel", b.name).Str("path", f.Path).Msg("channel bridge: file outside GOGOGOT_CHANNEL_INBOX ignored")
			continue
		}
		info, err := os.Stat(f.Path)
		if err != nil || info.Size() > maxInboundFile {
			log.Warn().Err(err).Str("channel", b.name).Str("path", f.Path).Msg("channel bridge: file missing or too large")
			continue
		}
		data, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		name := f.Name
		if name == "" {
			name = filepath.Base(f.Path)
		}
		mime := f.Mime
		if mime == "" {
			mime = "application/octet-stream"
		}
		atts = append(atts, transport.Attachment{Filename: name, MimeType: mime, Data: data})
	}

	b.mu.Lock()
	b.lastChat = string(in.Chat)
	b.mu.Unlock()

	select {
	case b.work <- render.Inbound{Chat: string(in.Chat), Text: in.Text, Attachments: atts}:
	default:
		log.Warn().Str("channel", b.name).Msg("channel bridge: inbound queue full, message dropped")
	}
}

// dispatchLoop hands inbound messages to the engine one at a time. It runs
// off the read loop because dispatching makes requests whose responses the
// read loop must deliver.
func (b *bridge) dispatchLoop(ctx, runCtx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-b.work:
			// The bridge may start before the engine (to fail fast on a bad
			// setup); hold messages until the engine takes them.
			h := b.handler()
			for h == nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(50 * time.Millisecond):
				}
				h = b.handler()
			}
			render.Dispatch(runCtx, h, b.surf, in)
		}
	}
}

func (b *bridge) inInbox(path string) bool {
	rel, err := filepath.Rel(b.inbox, filepath.Clean(path))
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

// settle waits for the bridge to report ready, returning early when it dies
// on startup so the caller sees the crash (and can read the logs) right away.
func (b *bridge) settle(timeout time.Duration) Status {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s := b.status(); s.State == StateRunning || s.State == StateCrashed {
			return s
		}
		time.Sleep(100 * time.Millisecond)
	}
	return b.status()
}

// waitReady blocks until the current process reported ready or timeout.
func (b *bridge) waitReady(timeout time.Duration) bool {
	b.mu.Lock()
	ready := b.ready
	b.mu.Unlock()
	select {
	case <-ready:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (b *bridge) call(ctx context.Context, timeout time.Duration, req map[string]any) (response, error) {
	b.mu.Lock()
	if b.stdin == nil {
		b.mu.Unlock()
		return response{}, fmt.Errorf("channel %s is not running", b.name)
	}
	b.nextID++
	id := b.nextID
	ch := make(chan response, 1)
	b.pending[id] = ch
	stdin := b.stdin
	b.mu.Unlock()

	req["id"] = id
	line, err := json.Marshal(req)
	if err != nil {
		return response{}, err
	}
	b.writeMu.Lock()
	_, err = stdin.Write(append(line, '\n'))
	b.writeMu.Unlock()
	if err != nil {
		b.forget(id)
		return response{}, fmt.Errorf("write to channel %s: %w", b.name, err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp := <-ch:
		if !resp.OK {
			if resp.Error == "" {
				resp.Error = "request failed"
			}
			return resp, errors.New(resp.Error)
		}
		return resp, nil
	case <-timer.C:
		b.forget(id)
		return response{}, fmt.Errorf("channel %s: %v timed out", b.name, req["op"])
	case <-ctx.Done():
		b.forget(id)
		return response{}, ctx.Err()
	}
}

func (b *bridge) forget(id int) {
	b.mu.Lock()
	delete(b.pending, id)
	b.mu.Unlock()
}

func (b *bridge) setState(s State, detail string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state != s {
		b.since = time.Now()
	}
	b.state, b.detail = s, detail
}

func (b *bridge) status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Status{Name: b.name, State: b.state, Detail: b.detail, PID: b.pid, Restarts: b.restarts, Since: b.since}
}

func (b *bridge) recentLogs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.logs...)
}

// chatForOwner is where owner notifications go: the chat the bridge named in
// its ready event, else the last chat the owner wrote from, else the owner ID
// (a private chat on most platforms).
func (b *bridge) chatForOwner() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case b.ownerChat != "":
		return b.ownerChat
	case b.lastChat != "":
		return b.lastChat
	default:
		return b.man.owner()
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// running reports whether a process is up to take requests.
func (b *bridge) running() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stdin != nil
}

// --- render.Surface ---

func (b *bridge) Caps() render.Caps {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.man.caps()
}

func (b *bridge) Send(ctx context.Context, chat string, t render.Text) (string, error) {
	resp, err := b.call(ctx, requestTimeout, map[string]any{
		"op": "send", "chat": chat, "text": t.Body, "plain": t.Plain, "status": t.Status,
	})
	return string(resp.MsgID), err
}

func (b *bridge) Edit(ctx context.Context, chat, msgID string, t render.Text) error {
	_, err := b.call(ctx, requestTimeout, map[string]any{
		"op": "edit", "chat": chat, "msg_id": msgID, "text": t.Body, "plain": t.Plain, "status": t.Status,
	})
	return err
}

func (b *bridge) Delete(ctx context.Context, chat, msgID string) error {
	_, err := b.call(ctx, requestTimeout, map[string]any{"op": "delete", "chat": chat, "msg_id": msgID})
	return err
}

func (b *bridge) Typing(ctx context.Context, chat string) error {
	_, err := b.call(ctx, requestTimeout, map[string]any{"op": "typing", "chat": chat})
	return err
}

func (b *bridge) SendFile(ctx context.Context, chat, path, caption string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	_, err = b.call(ctx, fileTimeout, map[string]any{"op": "send_file", "chat": chat, "path": abs, "caption": caption})
	return err
}

func (b *bridge) Ask(ctx context.Context, chat string, t render.Text, options []transport.AskOption) error {
	opts := make([]map[string]string, len(options))
	for i, o := range options {
		opts[i] = map[string]string{"value": o.Value, "label": o.Label}
	}
	_, err := b.call(ctx, requestTimeout, map[string]any{
		"op": "ask", "chat": chat, "text": t.Body, "plain": t.Plain, "options": opts,
	})
	return err
}
