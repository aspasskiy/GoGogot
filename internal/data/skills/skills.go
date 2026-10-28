// Package skills ships the skills bundled with GoGogot (the telegram channel
// and the add-channel guide) and installs them into the skills directory.
package skills

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
)

//go:embed all:bundled
var bundled embed.FS

const (
	// hashFile records the hash of each bundled file as last installed, so an
	// upgrade can tell which files the agent has modified since.
	hashFile = ".bundled.json"
	// upstreamDir receives newer bundled versions of files the agent
	// modified, for it to merge by hand.
	upstreamDir = ".upstream"
	// brokenDir keeps the agent's files that Restore replaced.
	brokenDir = ".broken"
)

// record is the hash file: which bundled skill a directory holds and the
// hash of each file as last installed.
type record struct {
	Source string            `json:"source"`
	Files  map[string]string `json:"files"`
}

// Install copies every bundled skill into skillsDir, file by file, like
// package conffiles:
//   - a missing skill is installed;
//   - a file unchanged since the last install is upgraded to the bundled one;
//   - a file the agent modified is left alone, the new version goes to
//     .upstream/ in the skill;
//   - if the agent already has its own skill with the same name, the bundled
//     one is installed as <name>-bundled instead.
//
// It returns the directory each bundled skill lives in, by bundled name.
func Install(skillsDir string) (map[string]string, error) {
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return nil, err
	}
	entries, err := bundled.ReadDir("bundled")
	if err != nil {
		return nil, err
	}
	installed := map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir, ok := placeFor(skillsDir, e.Name())
		if !ok {
			log.Warn().Str("skill", e.Name()).Msg("bundled skill not installed: its name and fallback name are taken")
			continue
		}
		if installOne(skillsDir, e.Name(), dir) {
			installed[e.Name()] = dir
		}
	}
	return installed, nil
}

// placeFor picks the directory for a bundled skill: its own name unless the
// agent owns a skill there, then <name>-bundled.
func placeFor(skillsDir, name string) (string, bool) {
	for _, dir := range []string{name, name + "-bundled"} {
		rec, exists, err := readRecord(filepath.Join(skillsDir, dir))
		if !exists || (err == nil && rec.Source == name) {
			if dir != name {
				log.Warn().Str("skill", name).Str("dir", dir).Msg("a skill with this name already exists; bundled skill installed under another name")
			}
			return dir, true
		}
	}
	return "", false
}

// readRecord loads dir's hash file. exists is false when dir does not exist.
func readRecord(dir string) (rec record, exists bool, err error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return rec, false, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, hashFile))
	if err != nil {
		return rec, true, err
	}
	return rec, true, json.Unmarshal(data, &rec)
}

func installOne(skillsDir, name, dirName string) bool {
	logger := log.With().Str("skill", dirName).Logger()
	dst := filepath.Join(skillsDir, dirName)

	files, err := bundledFiles(name, dirName)
	if err != nil {
		logger.Error().Err(err).Msg("bundled skill: read failed")
		return false
	}
	rec, _, _ := readRecord(dst)

	next := record{Source: name, Files: map[string]string{}}
	var upgraded, kept []string
	for _, rel := range sortedKeys(files) {
		data := files[rel]
		p := filepath.Join(dst, filepath.FromSlash(rel))
		cur, err := os.ReadFile(p)
		switch {
		case err == nil && bytes.Equal(cur, data):
			next.Files[rel] = sum(data)
		case os.IsNotExist(err) || (err == nil && sum(cur) == rec.Files[rel]):
			if err := writeFile(p, data); err != nil {
				logger.Error().Err(err).Str("file", rel).Msg("bundled skill: write failed")
				return false
			}
			next.Files[rel] = sum(data)
			upgraded = append(upgraded, rel)
		default:
			// Modified locally: keep it, offer the new version alongside.
			if err := writeFile(filepath.Join(dst, upstreamDir, filepath.FromSlash(rel)), data); err != nil {
				logger.Error().Err(err).Str("file", rel).Msg("bundled skill: write .upstream failed")
			}
			next.Files[rel] = rec.Files[rel]
			kept = append(kept, rel)
		}
	}
	// Files dropped from the bundle go too, unless the agent changed them.
	for rel, h := range rec.Files {
		if _, ok := files[rel]; ok {
			continue
		}
		p := filepath.Join(dst, filepath.FromSlash(rel))
		if cur, err := os.ReadFile(p); err == nil && sum(cur) == h {
			_ = os.Remove(p)
		}
	}
	if len(kept) == 0 {
		_ = os.RemoveAll(filepath.Join(dst, upstreamDir))
	}

	if err := writeRecord(dst, next); err != nil {
		logger.Error().Err(err).Msg("bundled skill: write " + hashFile + " failed")
		return false
	}
	if len(upgraded) > 0 {
		logger.Info().Strs("files", upgraded).Msg("bundled skill installed")
	}
	if len(kept) > 0 {
		logger.Info().Strs("files", kept).Msg("bundled skill: locally modified files kept, new versions in " + upstreamDir)
	}
	return true
}

func writeRecord(dir string, rec record) error {
	data, _ := json.MarshalIndent(rec, "", "  ")
	return os.WriteFile(filepath.Join(dir, hashFile), append(data, '\n'), 0o644)
}

// Restore puts the stock version of a bundled skill back into skillsDir/dir,
// moving every modified file to .broken/ first. It returns the restored files
// (none when dir is not a bundled skill or is unmodified).
func Restore(skillsDir, dir string) ([]string, error) {
	dst := filepath.Join(skillsDir, dir)
	rec, exists, err := readRecord(dst)
	if !exists || err != nil || rec.Source == "" {
		return nil, nil
	}
	files, err := bundledFiles(rec.Source, dir)
	if err != nil {
		return nil, err
	}

	var restored []string
	for _, rel := range sortedKeys(files) {
		p := filepath.Join(dst, filepath.FromSlash(rel))
		cur, err := os.ReadFile(p)
		if err == nil && bytes.Equal(cur, files[rel]) {
			continue
		}
		if err == nil {
			if err := writeFile(filepath.Join(dst, brokenDir, filepath.FromSlash(rel)), cur); err != nil {
				return restored, err
			}
		}
		if err := writeFile(p, files[rel]); err != nil {
			return restored, err
		}
		rec.Files[rel] = sum(files[rel])
		restored = append(restored, rel)
	}
	if len(restored) > 0 {
		if err := writeRecord(dst, rec); err != nil {
			return restored, err
		}
	}
	return restored, nil
}

// bundledFiles maps relative path → content for the embedded skill name as
// installed into dirName: under a fallback dir its SKILL.md is renamed too, so
// the agent does not see two skills with one name.
func bundledFiles(name, dirName string) (map[string][]byte, error) {
	root := path.Join("bundled", name)
	out := map[string][]byte{}
	err := fs.WalkDir(bundled, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := bundled.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		if rel == "SKILL.md" && dirName != name {
			data = bytes.Replace(data, []byte("\nname: "+name+"\n"), []byte("\nname: "+dirName+"\n"), 1)
		}
		out[rel] = data
		return nil
	})
	return out, err
}

func sortedKeys(m map[string][]byte) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// writeFile writes one skill file; scripts (with a shebang) are executable.
func writeFile(p string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if bytes.HasPrefix(data, []byte("#!")) {
		mode = 0o755
	}
	return os.WriteFile(p, data, mode)
}
