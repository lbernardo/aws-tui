package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Column describes a table column. Width 0 means the column shares the
// remaining horizontal space with the other flexible columns.
type Column struct {
	Title string
	Width int
}

// Row is a table line. ID is the resource identifier copied with "y" and
// Data carries the domain object behind the row.
type Row struct {
	ID    string
	Cells []string
	Data  any
}

// TagsFunc loads the tags of the resource behind a row.
type TagsFunc func(ctx context.Context, row Row) (map[string]string, error)

// Action binds a key to a handler. In tables, row is the selected row (nil
// when the table is empty); in detail views row is always nil.
type Action struct {
	Key key.Binding
	Run func(row *Row) tea.Cmd
}

// NewAction is a shortcut to build an Action bound to a single key.
func NewAction(k, help string, run func(row *Row) tea.Cmd) Action {
	return Action{Key: key.NewBinding(key.WithKeys(k), key.WithHelp(k, help)), Run: run}
}

// TableConfig configures a TableView.
type TableConfig struct {
	Title    string
	Columns  []Column
	Load     func(ctx context.Context) ([]Row, error)
	OnSelect func(row Row) tea.Cmd
	Actions  []Action
	// Tags, when set, enables "t" to show the tags of the selected row.
	Tags      TagsFunc
	EmptyText string
}

type tableLoadedMsg struct {
	id   int64
	rows []Row
	err  error
}

var (
	keySelect = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	keyFilter = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	keyReload = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
	keyCopy   = key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy id"))
	keyTags   = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "tags"))
)

// TableView is a generic, filterable and asynchronously loaded list of
// resources. Most service screens are just a TableConfig.
type TableView struct {
	id        int64
	cfg       TableConfig
	table     table.Model
	filter    textinput.Model
	filtering bool
	spinner   spinner.Model
	loading   bool
	err       error
	rows      []Row
	visible   []Row
	hasIDs    bool
	width     int
	height    int
}

func NewTable(cfg TableConfig) *TableView {
	if cfg.EmptyText == "" {
		cfg.EmptyText = "No items found."
	}

	t := table.New(table.WithFocused(true))
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(ColorMuted).
		BorderBottom(true).
		Bold(true)
	styles.Selected = styles.Selected.
		Foreground(lipgloss.Color("#232F3E")).
		Background(ColorAccent).
		Bold(false)
	t.SetStyles(styles)

	f := textinput.New()
	f.Prompt = "/ "
	f.PromptStyle = PromptStyle
	f.Placeholder = "type to filter"

	s := spinner.New(spinner.WithSpinner(spinner.Dot))
	s.Style = TitleStyle

	v := &TableView{id: nextID(), cfg: cfg, table: t, filter: f, spinner: s}
	v.table.SetColumns(v.columns(80))
	return v
}

func (v *TableView) Title() string { return v.cfg.Title }

func (v *TableView) Init() tea.Cmd { return v.reload() }

func (v *TableView) reload() tea.Cmd {
	v.loading = true
	v.err = nil
	id, load := v.id, v.cfg.Load
	return tea.Batch(v.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
		defer cancel()
		rows, err := load(ctx)
		return tableLoadedMsg{id: id, rows: rows, err: err}
	})
}

func (v *TableView) CapturingInput() bool { return v.filtering }

func (v *TableView) HandleBack() bool {
	if v.filter.Value() == "" {
		return false
	}
	v.filter.SetValue("")
	v.applyFilter()
	return true
}

func (v *TableView) KeyBindings() []key.Binding {
	var keys []key.Binding
	if v.cfg.OnSelect != nil {
		keys = append(keys, keySelect)
	}
	for _, a := range v.cfg.Actions {
		keys = append(keys, a.Key)
	}
	if v.hasIDs {
		keys = append(keys, keyCopy)
	}
	if v.cfg.Tags != nil {
		keys = append(keys, keyTags)
	}
	return append(keys, keyFilter, keyReload)
}

func (v *TableView) SetSize(width, height int) {
	v.width, v.height = width, height
	v.table.SetColumns(v.columns(width))
	v.table.SetWidth(width)
	v.table.SetHeight(max(height-1, 3)) // one line for the title/filter bar
	v.filter.Width = max(width-4, 10)
}

// columns distributes the available width between flexible columns.
func (v *TableView) columns(width int) []table.Column {
	fixed, flex := 0, 0
	for _, c := range v.cfg.Columns {
		fixed += c.Width + 2 // cell padding
		if c.Width == 0 {
			flex++
		}
	}
	share := 10
	if flex > 0 {
		share = max((width-fixed)/flex, 10)
	}
	cols := make([]table.Column, len(v.cfg.Columns))
	for i, c := range v.cfg.Columns {
		w := c.Width
		if w == 0 {
			w = share
		}
		cols[i] = table.Column{Title: c.Title, Width: w}
	}
	return cols
}

func (v *TableView) selected() *Row {
	i := v.table.Cursor()
	if i < 0 || i >= len(v.visible) {
		return nil
	}
	return &v.visible[i]
}

func (v *TableView) applyFilter() {
	term := strings.ToLower(strings.TrimSpace(v.filter.Value()))
	v.visible = v.visible[:0]
	for _, r := range v.rows {
		if term == "" || strings.Contains(strings.ToLower(strings.Join(r.Cells, " ")), term) {
			v.visible = append(v.visible, r)
		}
	}
	rows := make([]table.Row, len(v.visible))
	for i, r := range v.visible {
		rows[i] = r.Cells
	}
	v.table.SetRows(rows)
	if v.table.Cursor() >= len(rows) {
		v.table.SetCursor(max(len(rows)-1, 0))
	}
}

func (v *TableView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tableLoadedMsg:
		if msg.id != v.id {
			return v, nil
		}
		v.loading = false
		v.err = msg.err
		v.rows = msg.rows
		v.hasIDs = false
		for _, r := range v.rows {
			if r.ID != "" {
				v.hasIDs = true
				break
			}
		}
		v.applyFilter()
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
		return v.handleKey(msg)
	}
	return v, nil
}

func (v *TableView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	if v.filtering {
		switch msg.String() {
		case "esc":
			v.filtering = false
			v.filter.Blur()
			v.filter.SetValue("")
			v.applyFilter()
			return v, nil
		case "enter":
			v.filtering = false
			v.filter.Blur()
			return v, nil
		}
		var cmd tea.Cmd
		v.filter, cmd = v.filter.Update(msg)
		v.applyFilter()
		return v, cmd
	}

	switch {
	case key.Matches(msg, keyFilter):
		v.filtering = true
		return v, v.filter.Focus()
	case key.Matches(msg, keyReload):
		return v, v.reload()
	case key.Matches(msg, keySelect):
		if v.cfg.OnSelect != nil && !v.loading {
			if r := v.selected(); r != nil {
				return v, v.cfg.OnSelect(*r)
			}
		}
		return v, nil
	case key.Matches(msg, keyCopy):
		if r := v.selected(); r != nil && r.ID != "" {
			return v, Copy(r.ID)
		}
		return v, nil
	case key.Matches(msg, keyTags):
		r := v.selected()
		if v.cfg.Tags == nil || r == nil || v.loading {
			return v, nil
		}
		row, load := *r, v.cfg.Tags
		return v, Push(NewTagsView(row.ID, func(ctx context.Context) (map[string]string, error) { return load(ctx, row) }))
	}

	for _, a := range v.cfg.Actions {
		if key.Matches(msg, a.Key) {
			if v.loading {
				return v, nil
			}
			return v, a.Run(v.selected())
		}
	}

	var cmd tea.Cmd
	v.table, cmd = v.table.Update(msg)
	return v, cmd
}

func (v *TableView) View() string {
	var bar string
	switch {
	case v.filtering || v.filter.Value() != "":
		bar = v.filter.View()
	default:
		bar = TitleStyle.Render(v.cfg.Title) + MutedStyle.Render(fmt.Sprintf("  %d items", len(v.rows)))
	}

	var body string
	switch {
	case v.loading:
		body = "\n " + v.spinner.View() + " Loading…"
	case v.err != nil:
		body = "\n " + ErrorStyle.Render("Error: "+v.err.Error()) + "\n\n " + MutedStyle.Render("press r to retry")
	case len(v.rows) == 0:
		body = "\n " + MutedStyle.Render(v.cfg.EmptyText)
	case len(v.visible) == 0:
		body = "\n " + MutedStyle.Render("No items match the filter.")
	default:
		body = v.table.View()
	}
	return bar + "\n" + body
}
