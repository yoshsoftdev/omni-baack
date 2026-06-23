package planlimits

// MemberLimit returns the max team members for a plan type string.
func MemberLimit(planType string) int {
	switch planType {
	case "starter":
		return 5
	case "growth":
		return 25
	case "business":
		return 100
	case "enterprise":
		return 0
	default:
		return 5
	}
}
