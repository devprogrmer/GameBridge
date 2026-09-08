package panel

// Phase17 permission middleware foundation.
func hasPermission(role string, permission string) bool {
	return role != "" && permission != ""
}
