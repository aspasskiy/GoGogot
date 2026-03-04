package agent

// DefaultCompaction returns sensible compaction defaults for use in AgentConfig.
func DefaultCompaction() CompactionConfig {
	return DefaultCompactionConfig()
}
