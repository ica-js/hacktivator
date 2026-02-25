package ui

import (
	"fmt"
	"strings"

	"github.com/ica-js/hacktivator/internal/azure"
)

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

// RenderRolesTable renders a styled table of role assignments.
// When includeStatus is true, an extra STATUS column is appended.
func RenderRolesTable(roles []azure.RoleAssignment, includeStatus bool) string {
	var header, divider string
	if includeStatus {
		header = fmt.Sprintf("  %-30s %-40s %-15s %-10s", "ROLE", "SCOPE", "TYPE", "STATUS")
		divider = "  " + strings.Repeat("─", 95)
	} else {
		header = fmt.Sprintf("  %-30s %-40s %-15s", "ROLE", "SCOPE", "TYPE")
		divider = "  " + strings.Repeat("─", 85)
	}

	var b strings.Builder
	b.WriteString(TitleStyle.Render(header) + "\n")
	b.WriteString(SubtleStyle.Render(divider) + "\n")

	for _, role := range roles {
		roleName := truncate(role.RoleName, 28)
		scopeName := truncate(role.ScopeName, 38)
		if includeStatus {
			status := role.Status
			if status == "" {
				status = "Active"
			}
			row := fmt.Sprintf("  %-30s %-40s %-15s %-10s", roleName, scopeName, role.ScopeType, status)
			b.WriteString(row + "\n")
		} else {
			row := fmt.Sprintf("  %-30s %-40s %-15s", roleName, scopeName, role.ScopeType)
			b.WriteString(row + "\n")
		}
	}

	return b.String()
}
