package local

import (
	"encoding/json"
	"fmt"
	"github.com/aspasskiy/gogogot/internal/tools/store"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *LocalStore) NewEpisode() *store.Episode {
	now := time.Now()
	ep := &store.Episode{
		ID:        uuid.NewString(),
		Status:    "active",
		StartedAt: now,
		UpdatedAt: now,
	}
	ep.SetPersister(s)
	return ep
}

func (s *LocalStore) episodePath(ep *store.Episode) string {
	date := ep.StartedAt.Format("2006-01-02")
	return filepath.Join(s.episodesDir(), date, ep.ID+".json")
}

func (s *LocalStore) messagesPath(ep *store.Episode) string {
	date := ep.StartedAt.Format("2006-01-02")
	return filepath.Join(s.episodesDir(), date, ep.ID+".messages.jsonl")
}

func (s *LocalStore) SaveEpisode(ep *store.Episode) error {
	p := s.episodePath(ep)
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ep.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(ep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func (s *LocalStore) LoadEpisode(id string) (*store.Episode, error) {
	fname := id + ".json"
	entries, err := os.ReadDir(s.episodesDir())
	if err != nil {
		return nil, fmt.Errorf("episode %q not found", id)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.episodesDir(), entry.Name(), fname))
		if err != nil {
			continue
		}
		var ep store.Episode
		if err := json.Unmarshal(data, &ep); err != nil {
			return nil, err
		}
		ep.SetPersister(s)
		return &ep, nil
	}
	return nil, fmt.Errorf("episode %q not found", id)
}

func (s *LocalStore) ListEpisodes() ([]store.EpisodeInfo, error) {
	dateDirs, err := os.ReadDir(s.episodesDir())
	if err != nil {
		return nil, err
	}
	var out []store.EpisodeInfo
	for _, dd := range dateDirs {
		if !dd.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.episodesDir(), dd.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".json") || strings.HasSuffix(f.Name(), ".messages.jsonl") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(s.episodesDir(), dd.Name(), f.Name()))
			if err != nil {
				continue
			}
			var ep store.Episode
			if json.Unmarshal(data, &ep) == nil {
				out = append(out, store.EpisodeInfo{
					ID:        ep.ID,
					Title:     ep.Title,
					Summary:   ep.Summary,
					Tags:      ep.Tags,
					Status:    ep.Status,
					StartedAt: ep.StartedAt,
					EndedAt:   ep.EndedAt,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}
