package agent

import "gogogot/core/agent/session"

// DefaultCompaction returns sensible compaction defaults for use in AgentConfig.
func DefaultCompaction() session.CompactionConfig {
	return session.DefaultCompactionConfig()
}
