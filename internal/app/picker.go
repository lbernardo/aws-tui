package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lbenardo/aws-tui/internal/ui"
)

const pickerMaxItems = 8

type pickerItem struct {
	Value string // what is returned on selection (and completed by tab)
	Title string
	Desc  string
}

// picker is a filterable prompt (k9s/vim ":" style) shown below the header,
// used to switch service, profile and region.
type picker struct {
	input       textinput.Model
	items       []pickerItem
	matches     []pickerItem
	cursor      int
	allowCustom bool // enter accepts a typed value that matches no item
	onSelect    func(value string) tea.Cmd
}

func newPicker(prompt, placeholder string, items []pickerItem, current string, allowCustom bool, onSelect func(string) tea.Cmd) *picker {
	in := textinput.New()
	in.Prompt = prompt
	in.PromptStyle = ui.PromptStyle
	in.Placeholder = placeholder
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Focus()
	p := &picker{input: in, items: items, allowCustom: allowCustom, onSelect: onSelect}
	p.filter()
	for i, it := range p.matches {
		if it.Value == current {
			p.cursor = i
		}
	}
	return p
}

// filter ranks items: exact value, then value/title prefix, then substring
// anywhere in value, title or description.
func (p *picker) filter() {
	term := strings.ToLower(strings.TrimSpace(p.input.Value()))
	var exact, prefix, contains []pickerItem
	for _, it := range p.items {
		value, title := strings.ToLower(it.Value), strings.ToLower(it.Title)
		switch {
		case term == "":
			contains = append(contains, it)
		case value == term:
			exact = append(exact, it)
		case strings.HasPrefix(value, term) || strings.HasPrefix(title, term):
			prefix = append(prefix, it)
		case strings.Contains(value+" "+title+" "+strings.ToLower(it.Desc), term):
			contains = append(contains, it)
		}
	}
	p.matches = append(append(exact, prefix...), contains...)
	p.cursor = min(p.cursor, max(len(p.matches)-1, 0))
}

// update handles a key while the picker is open. done means it must close.
func (p *picker) update(msg tea.KeyMsg) (done bool, cmd tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+g":
		return true, nil
	case "enter":
		if len(p.matches) > 0 {
			return true, p.onSelect(p.matches[p.cursor].Value)
		}
		if v := strings.TrimSpace(p.input.Value()); p.allowCustom && v != "" {
			return true, p.onSelect(v)
		}
		return false, nil
	case "up", "ctrl+p", "shift+tab":
		if p.cursor > 0 {
			p.cursor--
		}
		return false, nil
	case "down", "ctrl+n":
		if p.cursor < len(p.matches)-1 {
			p.cursor++
		}
		return false, nil
	case "tab":
		if len(p.matches) > 0 {
			p.input.SetValue(p.matches[p.cursor].Value)
			p.input.CursorEnd()
			p.filter()
		}
		return false, nil
	case "backspace":
		if p.input.Value() == "" {
			return true, nil // like vim: backspace on an empty prompt cancels
		}
	}
	p.input, cmd = p.input.Update(msg)
	p.filter()
	return false, cmd
}

func (p *picker) view(width int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.ColorAccent).
		Padding(0, 1).
		Width(max(width-2, 20))

	valueW, titleW := 0, 0
	for _, it := range p.items {
		valueW = max(valueW, lipgloss.Width(it.Value))
		titleW = max(titleW, lipgloss.Width(it.Title))
	}
	valueW, titleW = min(valueW+2, 32), min(titleW+2, 32)
	if titleW == 2 {
		titleW = 0
	}

	lines := []string{p.input.View()}
	if len(p.matches) == 0 {
		hint := "no matches"
		if p.allowCustom && p.input.Value() != "" {
			hint += ", enter uses the typed value"
		}
		lines = append(lines, ui.MutedStyle.Render(hint))
	}

	// Keep the cursor visible when there are more matches than rows.
	start := max(p.cursor-pickerMaxItems+1, 0)
	end := min(start+pickerMaxItems, len(p.matches))
	selected := lipgloss.NewStyle().Foreground(lipgloss.Color("#232F3E")).Background(ui.ColorAccent)
	for i := start; i < end; i++ {
		it := p.matches[i]
		line := lipgloss.NewStyle().Width(valueW).Render(it.Value)
		if titleW > 0 {
			line += lipgloss.NewStyle().Width(titleW).Render(it.Title)
		}
		line += it.Desc
		if i == p.cursor {
			line = selected.Width(max(width-6, 10)).Render(line)
		} else {
			line = ui.MutedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	if more := len(p.matches) - end; more > 0 {
		lines = append(lines, ui.MutedStyle.Render(fmt.Sprintf("… %d more", more)))
	}
	return box.Render(strings.Join(lines, "\n"))
}
