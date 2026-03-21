package local

import (
	"encoding/json"
	"fmt"
	"gogogot/internal/tools/store"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *LocalStore) NewChat() *store.Chat {
	now := time.Now()
	ch := &store.Chat{
		ID:        uuid.NewString(),
		Status:    "active",
		StartedAt: now,
		UpdatedAt: now,
	}
	ch.SetPersister(s)
	return ch
}

func (s *LocalStore) chatPath(ch *store.Chat) string {
	date := ch.StartedAt.Format("2006-01-02")
	return filepath.Join(s.chatsDir(), date, ch.ID+".json")
}

func (s *LocalStore) messagesPath(ch *store.Chat) string {
	date := ch.StartedAt.Format("2006-01-02")
	return filepath.Join(s.chatsDir(), date, ch.ID+".messages.jsonl")
}

func (s *LocalStore) SaveChat(ch *store.Chat) error {
	p := s.chatPath(ch)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ch.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(ch, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func (s *LocalStore) LoadChat(id string) (*store.Chat, error) {
	fname := id + ".json"
	entries, err := os.ReadDir(s.chatsDir())
	if err != nil {
		return nil, fmt.Errorf("chat %q not found", id)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.chatsDir(), entry.Name(), fname))
		if err != nil {
			continue
		}
		var ch store.Chat
		if err := json.Unmarshal(data, &ch); err != nil {
			return nil, err
		}
		ch.SetPersister(s)
		return &ch, nil
	}
	return nil, fmt.Errorf("chat %q not found", id)
}

func (s *LocalStore) ListChats() ([]store.ChatInfo, error) {
	dateDirs, err := os.ReadDir(s.chatsDir())
	if err != nil {
		return nil, err
	}
	var out []store.ChatInfo
	for _, dd := range dateDirs {
		if !dd.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.chatsDir(), dd.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".json") || strings.HasSuffix(f.Name(), ".messages.jsonl") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(s.chatsDir(), dd.Name(), f.Name()))
			if err != nil {
				continue
			}
			var ch store.Chat
			if json.Unmarshal(data, &ch) == nil {
				out = append(out, store.ChatInfo{
					ID:        ch.ID,
					Title:     ch.Title,
					Summary:   ch.Summary,
					Tags:      ch.Tags,
					Status:    ch.Status,
					StartedAt: ch.StartedAt,
					EndedAt:   ch.EndedAt,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}
