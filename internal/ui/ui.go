package ui

import (
	"fmt"
	"io"
	"strings"
)

func PrintTable(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	for _, row := range rows {
		for i, col := range row {
			if len(col) > widths[i] {
				widths[i] = len(col)
			}
		}
	}

	writeRow(w, headers, widths)
	writeRow(w, make([]string, len(headers)), widths)

	for _, row := range rows {
		writeRow(w, row, widths)
	}
}

func writeRow(w io.Writer, cols []string, widths []int) {
	parts := make([]string, len(cols))
	for i, col := range cols {
		pad := widths[i] - len(col)
		parts[i] = col + strings.Repeat(" ", pad)
	}
	fmt.Fprintln(w, strings.Join(parts, "  "))
}
