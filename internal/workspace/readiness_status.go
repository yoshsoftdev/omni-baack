package workspace

func resolveReadinessStatus(score int) string {
	switch {
	case score == 0:
		return "not_started"
	case score == 100:
		return "ready"
	default:
		return "in_progress"
	}
}
