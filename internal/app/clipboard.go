package app

import (
	"fmt"
	"os"

	"github.com/atotto/clipboard"
	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbernardo/aws-tui/internal/ui"
)

// writeClipboard is a variable so tests do not touch the real clipboard.
var writeClipboard = clipboard.WriteAll

// copyToClipboard uses the system clipboard (pbcopy, xclip, wl-copy...) and
// falls back to the OSC 52 escape sequence, which also works over SSH in
// most modern terminals.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		if err := writeClipboard(text); err != nil {
			if _, err := osc52.New(text).WriteTo(os.Stderr); err != nil {
				return ui.ErrorMsg{Err: fmt.Errorf("copy to clipboard: %w", err)}
			}
		}
		return ui.StatusMsg{Text: "Copied " + text}
	}
}
