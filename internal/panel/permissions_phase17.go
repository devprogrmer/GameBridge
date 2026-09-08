// SPDX-License-Identifier: AGPL-3.0-only
package panel

func hasPermission(role string, permission string) bool {
	return role != "" && permission != ""
}

func phase17DefaultPermissionCheck() bool {
	return hasPermission("admin", "panel.access")
}
