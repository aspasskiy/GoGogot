package local

import (
	"fmt"
	"github.com/aspasskiy/gogogot/internal/tools/store"
	"os"
	"path/filepath"
	"strings"
)

func (s *LocalStore) ListMemory() ([]store.MemoryFile, error) {
	entries, err := os.ReadDir(s.memoryDir())
	if err != nil {
		return nil, err
	}
	var out []store.MemoryFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, store.MemoryFile{Name: e.Name(), Size: info.Size()})
	}
	return out, nil
}

func (s *LocalStore) ReadMemory(filename string) (string, error) {
	safe := filepath.Base(filename)
	data, err := os.ReadFile(filepath.Join(s.memoryDir(), safe))
	if os.IsNotExist(err) {
		return "", fmt.Errorf("memory file %q not found", safe)
	}
	return string(data), err
}

func (s *LocalStore) WriteMemory(filename, content string) error {
	safe := filepath.Base(filename)
	if !strings.HasSuffix(safe, ".md") {
		safe += ".md"
	}
	return os.WriteFile(filepath.Join(s.memoryDir(), safe), []byte(content), 0o644)
}

func (s *LocalStore) DeleteMemory(filename string) error {
	safe := filepath.Base(filename)
	return os.Remove(filepath.Join(s.memoryDir(), safe))
}
