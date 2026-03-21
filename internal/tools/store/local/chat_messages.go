package local

import (
	"bufio"
	"encoding/json"
	"gogogot/internal/llm/types"
	"gogogot/internal/tools/store"
	"os"
	"time"

	"github.com/rs/zerolog/log"
)

type jsonMessage struct {
	Role      string               `json:"role"`
	Content   []types.ContentBlock `json:"content"`
	Timestamp time.Time            `json:"ts"`
	Compacted bool                 `json:"compacted,omitempty"`
}

func (s *LocalStore) LoadMessages(ch *store.Chat) error {
	msgs := make([]store.Turn, 0)
	f, err := os.Open(s.messagesPath(ch))
	if os.IsNotExist(err) {
		ch.SetMessages(msgs)
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

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
		msg := store.Turn{
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

func (s *LocalStore) AppendMessage(ch *store.Chat, msg store.Turn) {
	path := s.messagesPath(ch)
	if path == "" {
		return
	}
	line, err := json.Marshal(turnToJSON(msg))
	if err != nil {
		log.Error().Err(err).Msg("chat: failed to marshal message for JSONL")
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Error().Err(err).Msg("chat: failed to open JSONL for append")
		return
	}
	defer f.Close()
	f.Write(line)
	f.Write([]byte{'\n'})
}

func (s *LocalStore) ReplaceMessages(ch *store.Chat, msgs []store.Turn) error {
	path := s.messagesPath(ch)
	if path == "" {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, msg := range msgs {
		line, err := json.Marshal(turnToJSON(msg))
		if err != nil {
			continue
		}
		f.Write(line)
		f.Write([]byte{'\n'})
	}
	return nil
}

func (s *LocalStore) TextMessages(ch *store.Chat) ([]store.Message, error) {
	f, err := os.Open(s.messagesPath(ch))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var msgs []store.Message
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var jm struct {
			Role    string               `json:"role"`
			Content []types.ContentBlock `json:"content"`
		}
		if json.Unmarshal(line, &jm) != nil {
			continue
		}
		text := types.ExtractText(jm.Content)
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

func turnToJSON(msg store.Turn) jsonMessage {
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
