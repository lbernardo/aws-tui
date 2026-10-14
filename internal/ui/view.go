// Package ui contains the reusable building blocks of the TUI: the View
// contract, navigation/status messages and generic views (tables, details)
// that services compose to expose their resources.
package ui

import (
	"sync/atomic"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// View is a screen that can be pushed onto the navigation stack.
type View interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (View, tea.Cmd)
	View() string
	// Title is used in the breadcrumb.
	Title() string
	// KeyBindings are shown in the help footer.
	KeyBindings() []key.Binding
	// SetSize informs the space available for the view body.
	SetSize(width, height int)
}

// InputCapturer is implemented by views that may be capturing text input
// (e.g. a filter). While capturing, global key bindings are not applied.
type InputCapturer interface {
	CapturingInput() bool
}

// BackHandler is implemented by views that want to react to "back" (esc)
// before the app pops them, e.g. to clear a filter. Returning true consumes it.
type BackHandler interface {
	HandleBack() bool
}

var idSeq atomic.Int64

// nextID returns a unique id used to route async messages to their view.
func nextID() int64 { return idSeq.Add(1) }
