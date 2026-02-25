package ui

import "github.com/charmbracelet/lipgloss"

var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("5"))

	SuccessStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("2"))

	SubtleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	SpinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("5"))

	// Preview pane styles
	PreviewTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("5"))

	PreviewLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("7"))

	PreviewValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("7"))

)
