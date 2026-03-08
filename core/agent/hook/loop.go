package hook

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const (
	defaultLoopThreshold = 3
	maxHistorySize       = 30
)

type toolCallRecord struct {
	name     string
	argsHash string
	ts       time.Time
}

type LoopDetector struct {
	history   []toolCallRecord
	threshold int
}

func NewLoopDetector(threshold int) *LoopDetector {
	if threshold <= 0 {
		threshold = defaultLoopThreshold
	}
	return &LoopDetector{
		history:   make([]toolCallRecord, 0, maxHistorySize),
		threshold: threshold,
	}
}

// BeforeHook returns a hook that records the call and checks for loops.
func (ld *LoopDetector) BeforeHook() BeforeToolCallFunc {
	return func(_ context.Context, tc *ToolCallContext) error {
		h := hashArgs(tc.ArgsRaw)
		ld.record(tc.ToolName, h)
		if n := ld.consecutiveCount(); n >= ld.threshold {
			return fmt.Errorf(
				"Loop detected: %s called %d times with identical arguments. "+
					"Stop repeating and explain the situation to the user.",
				tc.ToolName, n,
			)
		}
		return nil
	}
}

func (ld *LoopDetector) record(name, argsHash string) {
	ld.history = append(ld.history, toolCallRecord{
		name: name, argsHash: argsHash, ts: time.Now(),
	})
	if len(ld.history) > maxHistorySize {
		ld.history = ld.history[len(ld.history)-maxHistorySize:]
	}
}

func (ld *LoopDetector) consecutiveCount() int {
	n := len(ld.history)
	if n == 0 {
		return 0
	}
	last := ld.history[n-1]
	count := 1
	for i := n - 2; i >= 0; i-- {
		r := ld.history[i]
		if r.name != last.name || r.argsHash != last.argsHash {
			break
		}
		count++
	}
	return count
}

func hashArgs(raw []byte) string {
	if len(raw) == 0 {
		return "empty"
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Sprintf("%x", sha256.Sum256(raw))
	}
	canonical := canonicalJSON(m)
	return fmt.Sprintf("%x", sha256.Sum256([]byte(canonical)))
}

func canonicalJSON(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(m))
	for _, k := range keys {
		out[k] = m[k]
	}
	b, _ := json.Marshal(out)
	return string(b)
}
