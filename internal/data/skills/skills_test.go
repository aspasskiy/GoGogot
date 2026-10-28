package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstallKeepsLocalChanges(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	bridge := filepath.Join(dir, "telegram", "bridge.py")
	skillMd := filepath.Join(dir, "telegram", "SKILL.md")
	stock := read(t, bridge)
	if info, _ := os.Stat(bridge); info.Mode()&0o100 == 0 {
		t.Error("bridge.py is not executable")
	}

	// The agent edits SKILL.md; a pretend older release left a stale bridge.py.
	if err := os.WriteFile(skillMd, []byte("my notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bridge, []byte("old release"), 0o755); err != nil {
		t.Fatal(err)
	}
	pretendInstalled(t, dir, "bridge.py", "old release")

	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	if got := read(t, bridge); got != stock {
		t.Error("unmodified bridge.py was not upgraded")
	}
	if got := read(t, skillMd); got != "my notes" {
		t.Error("modified SKILL.md was overwritten")
	}
	if _, err := os.Stat(filepath.Join(dir, "telegram", upstreamDir, "SKILL.md")); err != nil {
		t.Error("new SKILL.md was not offered in .upstream")
	}
}

func TestInstallBesideForeignSkill(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "telegram")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "SKILL.md"), []byte("the agent's own telegram notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed, err := Install(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(own, "bridge.py")); !os.IsNotExist(err) {
		t.Error("bundled files were written into a skill the agent owns")
	}
	if installed["telegram"] != "telegram-bundled" {
		t.Fatalf("installed = %v, want telegram under telegram-bundled", installed)
	}
	if md := read(t, filepath.Join(dir, "telegram-bundled", "SKILL.md")); !strings.Contains(md, "\nname: telegram-bundled\n") {
		t.Error("fallback SKILL.md still claims the taken name")
	}
	// Stable across restarts, and the renamed SKILL.md does not count as modified.
	if again, _ := Install(dir); again["telegram"] != "telegram-bundled" {
		t.Errorf("second install = %v", again)
	}
	if _, err := os.Stat(filepath.Join(dir, "telegram-bundled", upstreamDir)); !os.IsNotExist(err) {
		t.Error("renamed SKILL.md was treated as a local modification")
	}
}

func TestRestore(t *testing.T) {
	dir := t.TempDir()
	if _, err := Install(dir); err != nil {
		t.Fatal(err)
	}
	bridge := filepath.Join(dir, "telegram", "bridge.py")
	stock := read(t, bridge)
	if err := os.WriteFile(bridge, []byte("syntax error("), 0o755); err != nil {
		t.Fatal(err)
	}

	restored, err := Restore(dir, "telegram")
	if err != nil || len(restored) != 1 || restored[0] != "bridge.py" {
		t.Fatalf("Restore = %v, %v", restored, err)
	}
	if read(t, bridge) != stock || read(t, filepath.Join(dir, "telegram", brokenDir, "bridge.py")) != "syntax error(" {
		t.Error("stock file not restored or broken copy not kept")
	}
	if again, _ := Restore(dir, "telegram"); len(again) != 0 {
		t.Errorf("second Restore = %v, want nothing to do", again)
	}
	if none, _ := Restore(dir, "not-bundled"); len(none) != 0 {
		t.Errorf("Restore of a foreign skill = %v", none)
	}
}

// pretendInstalled rewrites the recorded hash of one file, as if content was
// what an earlier release installed.
func pretendInstalled(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, "telegram", hashFile)
	data := read(t, p)
	var rec record
	if err := json.Unmarshal([]byte(data), &rec); err != nil {
		t.Fatal(err)
	}
	rec.Files[rel] = sum([]byte(content))
	out, _ := json.Marshal(rec)
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
}
