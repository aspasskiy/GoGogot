package local

import (
	"bufio"
	"encoding/json"
	"github.com/aspasskiy/gogogot/internal/llm/types"
	"github.com/aspasskiy/gogogot/internal/tools/store"
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

func (s *LocalStore) LoadMessages(ep *store.Episode) error {
	msgs := make([]store.Turn, 0)
	f, err := os.Open(s.messagesPath(ep))
	if os.IsNotExist(err) {
		ep.SetMessages(msgs)
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
			log.Warn().Err(err).Msg("episode: skipping corrupt JSONL line")
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
	ep.SetMessages(msgs)
	return scanner.Err()
}

func (s *LocalStore) AppendMessage(ep *store.Episode, msg store.Turn) {
	path := s.messagesPath(ep)
	if path == "" {
		return
	}
	line, err := json.Marshal(turnToJSON(msg))
	if err != nil {
		log.Error().Err(err).Msg("episode: failed to marshal message for JSONL")
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Error().Err(err).Msg("episode: failed to open JSONL for append")
		return
	}
	defer f.Close()
	f.Write(line)
	f.Write([]byte{'\n'})
}

func (s *LocalStore) ReplaceMessages(ep *store.Episode, msgs []store.Turn) error {
	path := s.messagesPath(ep)
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

func (s *LocalStore) TextMessages(ep *store.Episode) ([]store.Message, error) {
	f, err := os.Open(s.messagesPath(ep))
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

func (s *LocalStore) HasMessages(ep *store.Episode) bool {
	info, err := os.Stat(s.messagesPath(ep))
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
