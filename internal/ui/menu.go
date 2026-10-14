package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// MenuItem is an entry of a Menu; Open builds the view pushed on enter.
type MenuItem struct {
	Name        string
	Description string
	Open        func() View
}

// NewMenu builds a static list of sub-screens, typically the resource types
// of a service (e.g. Instances, Volumes...).
func NewMenu(title string, items []MenuItem) *TableView {
	return NewTable(TableConfig{
		Title:   title,
		Columns: []Column{{Title: "Resource", Width: 20}, {Title: "Description"}},
		Load: func(context.Context) ([]Row, error) {
			rows := make([]Row, len(items))
			for i, it := range items {
				rows[i] = Row{Cells: []string{it.Name, it.Description}, Data: it}
			}
			return rows, nil
		},
		OnSelect: func(r Row) tea.Cmd { return Push(r.Data.(MenuItem).Open()) },
	})
}
