package app

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lbernardo/aws-tui/internal/service"
	"github.com/lbernardo/aws-tui/internal/ui"
)

// newHome lists every registered service.
func newHome() ui.View {
	return ui.NewTable(ui.TableConfig{
		Title: "Services",
		Columns: []ui.Column{
			{Title: "Service", Width: 14},
			{Title: "ID", Width: 10},
			{Title: "Description"},
		},
		Load: func(context.Context) ([]ui.Row, error) {
			var rows []ui.Row
			for _, s := range service.All() {
				rows = append(rows, ui.Row{Cells: []string{s.Name, s.ID, s.Description}, Data: s})
			}
			return rows, nil
		},
		OnSelect: func(r ui.Row) tea.Cmd {
			svc := r.Data.(service.Service)
			return func() tea.Msg { return openServiceMsg{svc: svc} }
		},
		EmptyText: "No services registered.",
	})
}
