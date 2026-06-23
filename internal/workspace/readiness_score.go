package workspace

func calculateReadinessScore(completed, total int) int {
	if completed <= 0 {
		return 0
	}
	if completed >= total {
		return 100
	}
	bucket := (completed*4 + total - 1) / total
	return bucket * 25
}
