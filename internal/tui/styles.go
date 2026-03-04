package tui

import (
	"charm.land/lipgloss/v2"
)

var (
	// Tab bar
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6600")).
			Padding(0, 1)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888")).
				Padding(0, 1)

	// Header
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	// Status/loading
	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Padding(0, 1)

	// Detail pane
	detailTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FF6600"))

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888"))

	detailValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF"))

	detailBorderStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#444444")).
				MarginLeft(1).
				PaddingLeft(1)

	// Help bar
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6600"))

	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888"))

	// Sort indicator
	sortActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6600"))

	// Filter
	filterPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF6600")).
				Bold(true)

	filterTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	filterModeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)

	// Flash message
	flashColor = lipgloss.Color("#00FF00")
	flashStyle = lipgloss.NewStyle().
			Foreground(flashColor).
			Bold(true)

	// Error
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Padding(1, 2)

	// Help overlay
	helpOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FF6600")).
				Padding(1, 2)

	helpOverlayTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FF6600"))

	helpOverlayKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF6600")).
				Width(16)

	helpOverlayDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CCCCCC"))
)
