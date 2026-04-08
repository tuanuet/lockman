package components

import (
	"fmt"
	"strings"
)

type Column struct {
	Title string
	Width int
}

func Table(columns []Column, rows [][]string, selected int) string {
	if len(rows) == 0 {
		headers := make([]string, len(columns))
		for i, c := range columns {
			headers[i] = fmt.Sprintf("%-*s", c.Width, c.Title)
		}
		return TitleStyle.Render(strings.Join(headers, "  "))
	}

	var lines []string
	headerCells := make([]string, len(columns))
	for i, c := range columns {
		headerCells[i] = fmt.Sprintf("%-*s", c.Width, c.Title)
	}
	lines = append(lines, TitleStyle.Render(strings.Join(headerCells, "  ")))

	for i, row := range rows {
		prefix := " "
		if i == selected {
			prefix = "▸ "
		}
		cells := make([]string, len(columns))
		for j, c := range columns {
			val := ""
			if j < len(row) {
				val = row[j]
			}
			cells[j] = fmt.Sprintf("%-*s", c.Width, val)
		}
		lines = append(lines, prefix+strings.Join(cells, "  "))
	}

	return strings.Join(lines, "\n")
}
