package local

import (
	"github.com/aspasskiy/gogogot/internal/tools/store"
	"os"
	"path/filepath"
)

// LocalStore implements store.Store using the local filesystem.
type LocalStore struct {
	dataDir string
}

var _ store.Store = (*LocalStore)(nil)

func New(dataDir string) (store.Store, error) {
	s := &LocalStore{dataDir: dataDir}
	if err := s.ensureDirs(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *LocalStore) DataDir() string  { return s.dataDir }
func (s *LocalStore) SkillsDir() string { return filepath.Join(s.dataDir, "skills") }

func (s *LocalStore) episodesDir() string       { return filepath.Join(s.dataDir, "episodes") }
func (s *LocalStore) memoryDir() string         { return filepath.Join(s.dataDir, "memory") }
func (s *LocalStore) activeEpisodePath() string { return filepath.Join(s.dataDir, "active_episode.txt") }

func (s *LocalStore) ensureDirs() error {
	for _, dir := range []string{s.episodesDir(), s.memoryDir(), s.SkillsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}
