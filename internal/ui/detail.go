package ui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Field is a name/value pair shown in a DetailView. A Field with an empty
// Name and Value renders as a blank separator line.
type Field struct {
	Name  string
	Value string
}

// DetailConfig configures a DetailView.
type DetailConfig struct {
	Title   string
	Load    func(ctx context.Context) ([]Field, error)
	Actions []Action
	// ID is the resource identifier copied with "y".
	ID string
	// Tags, when set, enables "t" to show the resource tags.
	Tags func(ctx context.Context) (map[string]string, error)
}

type detailLoadedMsg struct {
	id     int64
	fields []Field
	err    error
}

// DetailView shows the attributes of a single resource in a scrollable pane.
type DetailView struct {
	id       int64
	cfg      DetailConfig
	viewport viewport.Model
	spinner  spinner.Model
	loading  bool
	err      error
	fields   []Field
}

func NewDetail(cfg DetailConfig) *DetailView {
	s := spinner.New(spinner.WithSpinner(spinner.Dot))
	s.Style = TitleStyle
	return &DetailView{id: nextID(), cfg: cfg, viewport: viewport.New(80, 20), spinner: s}
}

func (v *DetailView) Title() string { return v.cfg.Title }

func (v *DetailView) Init() tea.Cmd { return v.reload() }

func (v *DetailView) reload() tea.Cmd {
	v.loading = true
	v.err = nil
	id, load := v.id, v.cfg.Load
	return tea.Batch(v.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
		defer cancel()
		fields, err := load(ctx)
		return detailLoadedMsg{id: id, fields: fields, err: err}
	})
}

func (v *DetailView) KeyBindings() []key.Binding {
	keys := make([]key.Binding, 0, len(v.cfg.Actions)+1)
	for _, a := range v.cfg.Actions {
		keys = append(keys, a.Key)
	}
	if v.cfg.ID != "" {
		keys = append(keys, keyCopy)
	}
	if v.cfg.Tags != nil {
		keys = append(keys, keyTags)
	}
	return append(keys, keyReload)
}

func (v *DetailView) SetSize(width, height int) {
	v.viewport.Width = width
	v.viewport.Height = max(height-2, 3)
	v.render()
}

func (v *DetailView) render() {
	nameWidth := 0
	for _, f := range v.fields {
		nameWidth = max(nameWidth, lipgloss.Width(f.Name))
	}
	valueStyle := lipgloss.NewStyle().Width(max(v.viewport.Width-nameWidth-4, 10))
	var b strings.Builder
	for _, f := range v.fields {
		if f.Name == "" && f.Value == "" {
			b.WriteString("\n")
			continue
		}
		name := LabelStyle.Width(nameWidth).Render(f.Name)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, " ", name, "  ", valueStyle.Render(f.Value)))
		b.WriteString("\n")
	}
	v.viewport.SetContent(b.String())
}

func (v *DetailView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case detailLoadedMsg:
		if msg.id != v.id {
			return v, nil
		}
		v.loading = false
		v.err = msg.err
		v.fields = msg.fields
		v.render()
		return v, nil

	case spinner.TickMsg:
		if !v.loading {
			return v, nil
		}
		var cmd tea.Cmd
		v.spinner, cmd = v.spinner.Update(msg)
		return v, cmd

	case RefreshMsg:
		return v, v.reload()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keyReload):
			return v, v.reload()
		case key.Matches(msg, keyCopy) && v.cfg.ID != "":
			return v, Copy(v.cfg.ID)
		case key.Matches(msg, keyTags) && v.cfg.Tags != nil:
			return v, Push(NewTagsView(v.cfg.ID, v.cfg.Tags))
		}
		for _, a := range v.cfg.Actions {
			if key.Matches(msg, a.Key) {
				return v, a.Run(nil)
			}
		}
		var cmd tea.Cmd
		v.viewport, cmd = v.viewport.Update(msg)
		return v, cmd
	}
	return v, nil
}

func (v *DetailView) View() string {
	bar := TitleStyle.Render(v.cfg.Title)
	switch {
	case v.loading:
		return bar + "\n\n " + v.spinner.View() + " Loading…"
	case v.err != nil:
		return bar + "\n\n " + ErrorStyle.Render("Error: "+v.err.Error()) + "\n\n " + MutedStyle.Render("press r to retry")
	}
	return bar + "\n\n" + v.viewport.View()
}
