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

// RenderEligibleRolesTable renders a styled table of eligible roles.
func RenderEligibleRolesTable(roles []azure.EligibleRole) string {
	header := fmt.Sprintf("  %-30s %-40s %-15s", "ROLE", "SCOPE", "TYPE")
	divider := "  " + strings.Repeat("─", 85)

	var b strings.Builder
	b.WriteString(TitleStyle.Render(header) + "\n")
	b.WriteString(SubtleStyle.Render(divider) + "\n")

	for _, role := range roles {
		roleName := truncate(role.RoleName, 28)
		scopeName := truncate(role.ScopeName, 38)
		row := fmt.Sprintf("  %-30s %-40s %-15s", roleName, scopeName, role.ScopeType)
		b.WriteString(row + "\n")
	}

	return b.String()
}

// RenderActiveRolesTable renders a styled table of active roles.
func RenderActiveRolesTable(roles []azure.EligibleRole) string {
	header := fmt.Sprintf("  %-30s %-40s %-15s %-10s", "ROLE", "SCOPE", "TYPE", "STATUS")
	divider := "  " + strings.Repeat("─", 95)

	var b strings.Builder
	b.WriteString(TitleStyle.Render(header) + "\n")
	b.WriteString(SubtleStyle.Render(divider) + "\n")

	for _, role := range roles {
		roleName := truncate(role.RoleName, 28)
		scopeName := truncate(role.ScopeName, 38)
		status := role.Status
		if status == "" {
			status = "Active"
		}
		row := fmt.Sprintf("  %-30s %-40s %-15s %-10s", roleName, scopeName, role.ScopeType, status)
		b.WriteString(row + "\n")
	}

	return b.String()
}
