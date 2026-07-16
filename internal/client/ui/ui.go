// Package ui holds shared styling and small render helpers used by both the
// filez CLI and the filezui console UI (lipgloss based).
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Brand credit shown across the console tools.
const Brand = "Filez — made with ♥ by DasDarki (github.com/DasDarki)"

// Palette.
var (
	Accent = lipgloss.Color("#8a7dff")
	Green  = lipgloss.Color("#37c98b")
	Red    = lipgloss.Color("#ff6178")
	Amber  = lipgloss.Color("#f6b352")
	Dim    = lipgloss.Color("#8a8fa3")
)

// Styles.
var (
	Title    = lipgloss.NewStyle().Bold(true).Foreground(Accent)
	Success  = lipgloss.NewStyle().Bold(true).Foreground(Green)
	Error    = lipgloss.NewStyle().Bold(true).Foreground(Red)
	Warn     = lipgloss.NewStyle().Foreground(Amber)
	Subtle   = lipgloss.NewStyle().Foreground(Dim)
	KeyStyle = lipgloss.NewStyle().Bold(true).Foreground(Accent)
	Label    = lipgloss.NewStyle().Bold(true)
	Box      = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Accent).
			Padding(0, 2)
)

// Logo returns the small Filez wordmark.
func Logo() string {
	f := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).
		Background(Accent).Padding(0, 1).Render("F")
	return f + " " + Title.Render("Filez")
}

// ProgressBar renders a [====>   ] style bar for a fraction in [0,1].
func ProgressBar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac * float64(width))
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return lipgloss.NewStyle().Foreground(Accent).Render(bar)
}

// HumanBytes formats a byte count.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
