package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// DefaultTimeout bounds every AWS call started by the UI.
const DefaultTimeout = 2 * time.Minute

// PushMsg opens a new view on top of the navigation stack.
type PushMsg struct{ View View }

// PopMsg goes back to the previous view.
type PopMsg struct{}

// StatusMsg shows an informative message in the footer.
type StatusMsg struct{ Text string }

// ErrorMsg shows an error in the footer.
type ErrorMsg struct{ Err error }

// RefreshMsg asks the current view to reload its data.
type RefreshMsg struct{}

// ConfirmMsg asks a yes/no question; OnConfirm runs on "y".
type ConfirmMsg struct {
	Prompt    string
	OnConfirm tea.Cmd
}

// PromptMsg asks for a line of text; OnSubmit runs with the typed value.
type PromptMsg struct {
	Label    string
	Value    string
	OnSubmit func(value string) tea.Cmd
}

// CopyMsg asks the app to copy Text to the clipboard.
type CopyMsg struct{ Text string }

// TaskResultMsg is the outcome of a Task.
type TaskResultMsg struct {
	Status  string
	Err     error
	Refresh bool
}

func Push(v View) tea.Cmd { return func() tea.Msg { return PushMsg{View: v} } }

func Pop() tea.Cmd { return func() tea.Msg { return PopMsg{} } }

func Copy(text string) tea.Cmd { return func() tea.Msg { return CopyMsg{Text: text} } }

func Refresh() tea.Cmd { return func() tea.Msg { return RefreshMsg{} } }

func Status(format string, args ...any) tea.Cmd {
	text := fmt.Sprintf(format, args...)
	return func() tea.Msg { return StatusMsg{Text: text} }
}

func Error(err error) tea.Cmd { return func() tea.Msg { return ErrorMsg{Err: err} } }

func Confirm(prompt string, onConfirm tea.Cmd) tea.Cmd {
	return func() tea.Msg { return ConfirmMsg{Prompt: prompt, OnConfirm: onConfirm} }
}

func Prompt(label, value string, onSubmit func(string) tea.Cmd) tea.Cmd {
	return func() tea.Msg { return PromptMsg{Label: label, Value: value, OnSubmit: onSubmit} }
}

// Task runs fn in the background, showing `running` while it executes and
// the string returned by fn when it finishes. When refresh is true the
// current view reloads after a successful run.
func Task(running string, refresh bool, fn func(ctx context.Context) (string, error)) tea.Cmd {
	return tea.Batch(
		Status("%s", running),
		func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
			defer cancel()
			status, err := fn(ctx)
			return TaskResultMsg{Status: status, Err: err, Refresh: refresh}
		},
	)
}
