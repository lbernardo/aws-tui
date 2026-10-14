package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	ColorAccent = lipgloss.AdaptiveColor{Light: "#D97706", Dark: "#FF9900"}
	ColorMuted  = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#8B949E"}
	ColorError  = lipgloss.AdaptiveColor{Light: "#DC2626", Dark: "#F87171"}
	ColorOK     = lipgloss.AdaptiveColor{Light: "#059669", Dark: "#34D399"}

	LogoStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#232F3E")).Background(ColorAccent).Padding(0, 1)
	TitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	MutedStyle  = lipgloss.NewStyle().Foreground(ColorMuted)
	ErrorStyle  = lipgloss.NewStyle().Foreground(ColorError)
	OKStyle     = lipgloss.NewStyle().Foreground(ColorOK)
	LabelStyle  = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	PromptStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
)

// HumanBytes formats a byte count using binary units.
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
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// FormatTime formats a timestamp in local time, or "-" when zero.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// OrDash returns s, or "-" when it is blank.
func OrDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}
