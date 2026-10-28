package bridge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aspasskiy/gogogot/internal/channel"
	"github.com/aspasskiy/gogogot/internal/channel/bridge"
	bundledskills "github.com/aspasskiy/gogogot/internal/data/skills"
	"github.com/aspasskiy/gogogot/internal/transport"
)

// fakeBotAPI is just enough of the Telegram Bot API for bridge.py.
type fakeBotAPI struct {
	t *testing.T

	mu      sync.Mutex
	updates []map[string]any
	calls   []apiCall
	nextMsg int
}

type apiCall struct {
	Method string
	Params map[string]any
	File   string // uploaded file name, for multipart calls
	Data   string // uploaded file content
}

func (f *fakeBotAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/file/botTEST/") {
		_, _ = io.WriteString(w, "JPEGDATA")
		return
	}
	method := strings.TrimPrefix(r.URL.Path, "/botTEST/")

	call := apiCall{Method: method, Params: map[string]any{}}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			f.t.Errorf("%s: bad multipart: %v", method, err)
		}
		for k, v := range r.MultipartForm.Value {
			call.Params[k] = v[0]
		}
		for _, fhs := range r.MultipartForm.File {
			fh := fhs[0]
			file, _ := fh.Open()
			data, _ := io.ReadAll(file)
			call.File, call.Data = fh.Filename, string(data)
		}
	} else {
		_ = json.NewDecoder(r.Body).Decode(&call.Params)
	}

	var result any = true
	switch method {
	case "getMe":
		result = map[string]any{"id": 1, "username": "testbot"}
	case "getUpdates":
		f.mu.Lock()
		ups := f.updates
		f.updates = nil
		f.mu.Unlock()
		if len(ups) == 0 {
			time.Sleep(100 * time.Millisecond) // a short long-poll
			ups = []map[string]any{}
		}
		writeOK(w, ups)
		return
	case "getFile":
		result = map[string]any{"file_path": "photos/p.jpg"}
	case "sendMessage", "sendDocument", "sendPhoto":
		f.mu.Lock()
		f.nextMsg++
		result = map[string]any{"message_id": 100 + f.nextMsg}
		f.mu.Unlock()
	}

	f.mu.Lock()
	f.calls = append(f.calls, call)
	f.mu.Unlock()
	writeOK(w, result)
}

func writeOK(w http.ResponseWriter, result any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
}

func (f *fakeBotAPI) push(u map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u["update_id"] = len(f.calls) + len(f.updates) + 1000
	f.updates = append(f.updates, u)
}

// waitCall waits for the first call to method after skip earlier calls were
// already inspected, and returns it with its index.
func (f *fakeBotAPI) waitCall(t *testing.T, method string, from int) (apiCall, int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for i := from; i < len(f.calls); i++ {
			if f.calls[i].Method == method {
				c := f.calls[i]
				f.mu.Unlock()
				return c, i + 1
			}
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no %s call", method)
	return apiCall{}, 0
}

func needPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not installed")
	}
}

func startManager(t *testing.T, skillsDir string) (*bridge.Manager, chan channel.Message) {
	t.Helper()
	m := bridge.NewManager(skillsDir)
	msgs := make(chan channel.Message, 16)
	ctx, cancel := context.WithCancel(context.Background())
	m.Start(ctx) // started before returning, so Reload works right away
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = m.Run(ctx, func(_ context.Context, msg channel.Message) { msgs <- msg })
	}()
	t.Cleanup(func() { cancel(); <-done })
	return m, msgs
}

func waitMessage(t *testing.T, msgs chan channel.Message) channel.Message {
	t.Helper()
	select {
	case msg := <-msgs:
		return msg
	case <-time.After(10 * time.Second):
		t.Fatal("no message reached the handler")
		return channel.Message{}
	}
}

func TestTelegramBridge(t *testing.T) {
	needPython(t)

	api := &fakeBotAPI{t: t}
	srv := httptest.NewServer(api)
	defer srv.Close()

	t.Setenv("TELEGRAM_BOT_TOKEN", "TEST")
	t.Setenv("TELEGRAM_OWNER_ID", "42")
	t.Setenv("TELEGRAM_API_BASE", srv.URL)

	skillsDir := t.TempDir()
	if _, err := bundledskills.Install(skillsDir); err != nil {
		t.Fatal(err)
	}
	m, msgs := startManager(t, skillsDir)
	m.WaitReady(10 * time.Second)

	if s := m.List(); len(s) != 1 || s[0].Name != "telegram" || s[0].State != bridge.StateRunning {
		t.Fatalf("channels = %+v, want telegram running", s)
	}

	owner := map[string]any{"id": 42}
	stranger := map[string]any{"id": 7}
	chat := map[string]any{"id": 42}

	// A stranger is ignored; the owner gets through.
	api.push(map[string]any{"message": map[string]any{"message_id": 1, "from": stranger, "chat": chat, "text": "evil"}})
	api.push(map[string]any{"message": map[string]any{"message_id": 2, "from": owner, "chat": chat, "text": "hello"}})
	msg := waitMessage(t, msgs)
	if msg.Text != "hello" {
		t.Fatalf("handler got %q, want the owner's message", msg.Text)
	}

	// A run renders as a status message edited into the HTML answer.
	events := make(chan transport.Event, 4)
	events <- transport.ToolStartEvent{Name: "bash", Label: "Running", Detail: "ls"}
	events <- transport.DoneEvent{Text: "**hi**"}
	close(events)
	msg.Reply.ConsumeEvents(context.Background(), events, nil)

	status, next := api.waitCall(t, "sendMessage", 0)
	if status.Params["parse_mode"] != nil || status.Params["disable_notification"] != true {
		t.Errorf("status message params = %v, want plain and silent", status.Params)
	}
	edit, next := api.waitCall(t, "editMessageText", next)
	if edit.Params["text"] != "<b>hi</b>" || edit.Params["parse_mode"] != "HTML" || edit.Params["message_id"] != float64(101) {
		t.Errorf("final edit params = %v", edit.Params)
	}

	// Files go up as multipart uploads.
	path := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(path, []byte("numbers"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := msg.Reply.SendFile(context.Background(), path, "your report"); err != nil {
		t.Fatalf("SendFile: %v", err)
	}
	doc, next := api.waitCall(t, "sendDocument", next)
	if doc.File != "report.txt" || doc.Data != "numbers" || doc.Params["caption"] != "your report" {
		t.Errorf("sendDocument = %+v", doc)
	}

	// Choices become inline buttons, and a press comes back as the value.
	if err := msg.Reply.SendAsk(context.Background(), "Which?", transport.AskChoice,
		[]transport.AskOption{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}, {Value: "c", Label: "C"}}); err != nil {
		t.Fatalf("SendAsk: %v", err)
	}
	ask, next := api.waitCall(t, "sendMessage", next)
	if kb := fmt.Sprint(ask.Params["reply_markup"]); !strings.Contains(kb, "callback_data:c") {
		t.Errorf("ask keyboard = %s", kb)
	}
	api.push(map[string]any{"callback_query": map[string]any{"id": "cb1", "from": owner, "data": "c",
		"message": map[string]any{"message_id": 5, "chat": chat}}})
	if msg := waitMessage(t, msgs); msg.Text != "c" {
		t.Errorf("button press = %q, want c", msg.Text)
	}

	// Photos are downloaded into the inbox and arrive as attachments.
	api.push(map[string]any{"message": map[string]any{"message_id": 3, "from": owner, "chat": chat, "caption": "look",
		"photo": []map[string]any{{"file_id": "small", "file_size": 10}, {"file_id": "big", "file_size": 8}}}})
	msg = waitMessage(t, msgs)
	if msg.Text != "look" || len(msg.Attachments) != 1 || string(msg.Attachments[0].Data) != "JPEGDATA" ||
		msg.Attachments[0].MimeType != "image/jpeg" {
		t.Errorf("photo message = %q with %+v", msg.Text, msg.Attachments)
	}

	// Commands are answered by the core.
	api.push(map[string]any{"message": map[string]any{"message_id": 4, "from": owner, "chat": chat, "text": "/new"}})
	if msg := waitMessage(t, msgs); msg.Command == nil || msg.Command.Name != channel.CmdNewChat {
		t.Fatalf("command = %+v", msg.Command)
	}
	answer, _ := api.waitCall(t, "sendMessage", next)
	if !strings.Contains(fmt.Sprint(answer.Params["text"]), "New conversation started") {
		t.Errorf("command answer = %v", answer.Params["text"])
	}

	// Owner notifications go to the owner chat named in the ready event.
	if err := m.OwnerReplier().SendText(context.Background(), "scheduled"); err != nil {
		t.Fatalf("owner SendText: %v", err)
	}
}

// TestCoreChecksOwner runs a bridge that forwards everything, so only the
// core's owner check stands between a stranger and the agent.
func TestCoreChecksOwner(t *testing.T) {
	needPython(t)

	skillsDir := t.TempDir()
	dir := filepath.Join(skillsDir, "leaky")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "channel.json"), `{"command": ["python3", "bridge.py"], "owner": "1"}`)
	writeFile(t, filepath.Join(dir, "bridge.py"), `
import json, sys
def emit(o): print(json.dumps(o), flush=True)
emit({"event": "ready"})
emit({"event": "message", "chat": "c", "sender": "2", "text": "stranger"})
emit({"event": "message", "chat": 5, "sender": 1, "text": "owner"})
print("bridge up", file=sys.stderr, flush=True)
for line in sys.stdin:
    req = json.loads(line)
    emit({"id": req["id"], "ok": True, "msg_id": "1"})
`)

	m, msgs := startManager(t, skillsDir)
	// The owner's message uses numeric IDs, as bridges often pass them through.
	if msg := waitMessage(t, msgs); msg.Text != "owner" {
		t.Fatalf("first message = %q, want only the owner's", msg.Text)
	}
	select {
	case msg := <-msgs:
		t.Fatalf("unexpected message %q", msg.Text)
	case <-time.After(300 * time.Millisecond):
	}

	waitLog(t, m, "leaky", "bridge up")
}

func TestReloadAppliesChanges(t *testing.T) {
	needPython(t)

	skillsDir := t.TempDir()
	dir := filepath.Join(skillsDir, "echo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "channel.json"), `{"command": ["python3", "bridge.py"], "owner": "1", "enabled": false}`)
	writeFile(t, filepath.Join(dir, "bridge.py"), `import sys; sys.exit("boom")`)

	m, _ := startManager(t, skillsDir)
	if s := m.List(); len(s) != 1 || s[0].State != bridge.StateDisabled {
		t.Fatalf("channels = %+v, want echo disabled", s)
	}

	// Enabled but broken: the restart reports the crash right away.
	writeFile(t, filepath.Join(dir, "channel.json"), `{"command": ["python3", "bridge.py"], "owner": "1"}`)
	s, err := m.Reload("echo")
	if err != nil || s.State != bridge.StateCrashed {
		t.Fatalf("reload broken = %+v, %v; want crashed", s, err)
	}
	if logs, _ := m.Logs("echo"); len(logs) == 0 || logs[len(logs)-1] != "boom" {
		t.Errorf("logs = %q, want the crash message", logs)
	}

	// Fixed: running.
	writeFile(t, filepath.Join(dir, "bridge.py"), `
import json, sys
print(json.dumps({"event": "ready"}), flush=True)
for line in sys.stdin:
    pass
`)
	s, err = m.Reload("echo")
	if err != nil || s.State != bridge.StateRunning {
		t.Fatalf("reload fixed = %+v, %v; want running", s, err)
	}

	// Missing env keeps a channel idle instead of crashing it.
	writeFile(t, filepath.Join(dir, "channel.json"), `{"command": ["python3", "bridge.py"], "owner_env": "GOGOGOT_TEST_UNSET_OWNER"}`)
	s, err = m.Reload("echo")
	if err != nil || s.State != bridge.StateMissingEnv || !strings.Contains(s.Detail, "GOGOGOT_TEST_UNSET_OWNER") {
		t.Fatalf("reload missing env = %+v, %v", s, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRestartKeepsRunAlive is channel_restart called from a run on the same
// channel: the run must survive and its reply must go out through the new
// bridge process.
func TestRestartKeepsRunAlive(t *testing.T) {
	needPython(t)

	skillsDir := t.TempDir()
	dir := filepath.Join(skillsDir, "self")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bridgeV := func(v string) string {
		return `
import json, sys
def emit(o): print(json.dumps(o), flush=True)
emit({"event": "ready"})
if "` + v + `" == "v1":
    emit({"event": "message", "chat": "c", "sender": "1", "text": "restart yourself"})
for line in sys.stdin:
    req = json.loads(line)
    print("` + v + ` got " + req["op"] + ": " + req.get("text", ""), file=sys.stderr, flush=True)
    emit({"id": req["id"], "ok": True, "msg_id": "` + v + `"})
`
	}
	writeFile(t, filepath.Join(dir, "channel.json"), `{"command": ["python3", "bridge.py"], "owner": "1"}`)
	writeFile(t, filepath.Join(dir, "bridge.py"), bridgeV("v1"))

	m := bridge.NewManager(skillsDir)
	type delivered struct {
		ctx context.Context
		msg channel.Message
	}
	got := make(chan delivered, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = m.Run(ctx, func(ctx context.Context, msg channel.Message) { got <- delivered{ctx, msg} })
	}()
	t.Cleanup(func() { cancel(); <-done })

	var d delivered
	select {
	case d = <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("no message")
	}

	writeFile(t, filepath.Join(dir, "bridge.py"), bridgeV("v2"))
	if s, err := m.Reload("self"); err != nil || s.State != bridge.StateRunning {
		t.Fatalf("reload = %+v, %v", s, err)
	}
	if d.ctx.Err() != nil {
		t.Fatal("the run was cancelled by restarting its channel")
	}
	if err := d.msg.Reply.SendText(d.ctx, "restarted"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	waitLog(t, m, "self", "v2 got send: restarted")
}

// TestRollbackAfterCrashLoop breaks the bundled telegram bridge the way a bad
// edit would: the core must restore the stock bridge and get back online.
func TestRollbackAfterCrashLoop(t *testing.T) {
	needPython(t)

	api := &fakeBotAPI{t: t}
	srv := httptest.NewServer(api)
	defer srv.Close()
	t.Setenv("TELEGRAM_BOT_TOKEN", "TEST")
	t.Setenv("TELEGRAM_OWNER_ID", "42")
	t.Setenv("TELEGRAM_API_BASE", srv.URL)

	skillsDir := t.TempDir()
	if _, err := bundledskills.Install(skillsDir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(skillsDir, "telegram", "bridge.py"), "this is not python(")

	m := bridge.NewManager(skillsDir)
	m.Rollback = func(name string) ([]string, error) { return bundledskills.Restore(skillsDir, name) }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = m.Run(ctx, func(context.Context, channel.Message) {})
	}()
	t.Cleanup(func() { cancel(); <-done })

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if s, _ := m.Status("telegram"); s.State == bridge.StateRunning {
			logs, _ := m.Logs("telegram")
			if !strings.Contains(strings.Join(logs, "\n"), "restored stock bridge.py") {
				t.Errorf("logs do not mention the rollback: %q", logs)
			}
			if _, err := os.Stat(filepath.Join(skillsDir, "telegram", ".broken", "bridge.py")); err != nil {
				t.Error("broken bridge was not kept")
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	s, _ := m.Status("telegram")
	t.Fatalf("telegram did not come back: %+v", s)
}

// waitLog waits for a bridge's stderr to contain line. stderr is read apart
// from stdout, so a log line may arrive after the response it precedes.
func waitLog(t *testing.T, m *bridge.Manager, name, line string) {
	t.Helper()
	var logs []string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		logs, _ = m.Logs(name)
		for _, l := range logs {
			if l == line {
				return
			}
		}
	}
	t.Fatalf("%s logs = %q, want %q", name, logs, line)
}
