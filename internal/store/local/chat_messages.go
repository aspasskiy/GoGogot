package local

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aspasskiy/gogogot/internal/domain"
	"github.com/aspasskiy/gogogot/internal/store"

	"github.com/rs/zerolog/log"
)

type jsonMessage struct {
	Role      string                `json:"role"`
	Content   []domain.ContentBlock `json:"content"`
	Timestamp time.Time             `json:"ts"`
	Compacted bool                  `json:"compacted,omitempty"`
}

func (s *LocalStore) LoadMessages(ch *store.Chat) error {
	msgs := make([]domain.Turn, 0)
	f, err := os.Open(s.messagesPath(ch))
	if os.IsNotExist(err) {
		ch.SetMessages(msgs)
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var jm jsonMessage
		if err := json.Unmarshal(line, &jm); err != nil {
			log.Warn().Err(err).Msg("chat: skipping corrupt JSONL line")
			continue
		}
		msg := domain.Turn{
			Role:      jm.Role,
			Content:   jm.Content,
			Timestamp: jm.Timestamp,
		}
		if jm.Compacted {
			msg.Metadata = map[string]any{"compacted": true}
		}
		msgs = append(msgs, msg)
	}
	ch.SetMessages(msgs)
	return scanner.Err()
}

func (s *LocalStore) AppendMessage(ch *store.Chat, msg domain.Turn) error {
	path := s.messagesPath(ch)
	if path == "" {
		return nil
	}
	line, err := json.Marshal(turnToJSON(msg))
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open messages file: %w", err)
	}
	line = append(line, '\n')
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return fmt.Errorf("append message: %w", err)
	}
	return f.Close()
}

// ReplaceMessages rewrites the whole history. It writes to a temporary file
// and renames it over the original, so a crash mid-write never leaves a
// truncated history behind.
func (s *LocalStore) ReplaceMessages(ch *store.Chat, msgs []domain.Turn) error {
	path := s.messagesPath(ch)
	if path == "" {
		return nil
	}

	var buf bytes.Buffer
	for _, msg := range msgs {
		line, err := json.Marshal(turnToJSON(msg))
		if err != nil {
			return fmt.Errorf("marshal message: %w", err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return writeFileAtomic(path, buf.Bytes(), 0o644)
}

// writeFileAtomic writes data to a temp file in the same directory, syncs it
// and renames it over path.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *LocalStore) TextMessages(ch *store.Chat) ([]store.Message, error) {
	f, err := os.Open(s.messagesPath(ch))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var msgs []store.Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var jm struct {
			Role    string                `json:"role"`
			Content []domain.ContentBlock `json:"content"`
		}
		if json.Unmarshal(line, &jm) != nil {
			continue
		}
		text := domain.ExtractText(jm.Content)
		if text == "" {
			continue
		}
		msgs = append(msgs, store.Message{Role: jm.Role, Content: text})
	}
	return msgs, scanner.Err()
}

func (s *LocalStore) HasMessages(ch *store.Chat) bool {
	info, err := os.Stat(s.messagesPath(ch))
	return err == nil && info.Size() > 0
}

func turnToJSON(msg domain.Turn) jsonMessage {
	jm := jsonMessage{
		Role:      msg.Role,
		Content:   msg.Content,
		Timestamp: msg.Timestamp,
	}
	if v, ok := msg.Metadata["compacted"].(bool); ok && v {
		jm.Compacted = true
	}
	return jm
}
