package ui

import (
	"context"
	"sort"
)

// NewTagsView lists the tags of a resource; "y" copies the selected value.
func NewTagsView(resourceID string, load func(ctx context.Context) (map[string]string, error)) *TableView {
	title := "Tags"
	if resourceID != "" {
		title = "Tags: " + resourceID
	}
	return NewTable(TableConfig{
		Title:   title,
		Columns: []Column{{Title: "Key", Width: 32}, {Title: "Value"}},
		Load: func(ctx context.Context) ([]Row, error) {
			tags, err := load(ctx)
			if err != nil {
				return nil, err
			}
			keys := make([]string, 0, len(tags))
			for k := range tags {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			rows := make([]Row, len(keys))
			for i, k := range keys {
				rows[i] = Row{ID: tags[k], Cells: []string{k, tags[k]}}
			}
			return rows, nil
		},
		EmptyText: "This resource has no tags.",
	})
}
