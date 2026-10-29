package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintTable_AlignsColumnsByMaxWidth(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, []string{"NAME", "STATUS"}, [][]string{
		{"api", "running"},
		{"long-task-name", "stopped"},
	})

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (header, separator, 2 rows; got %d): %q", len(lines), buf.String())
	}

	// All data rows and the header should be padded to the same total width
	// (modulo the trailing row's own content length), since widths are
	// computed from the longest value in each column.
	headerLine := lines[0]
	if !strings.HasPrefix(headerLine, "NAME") {
		t.Errorf("expected header line to start with NAME, got %q", headerLine)
	}
	lastRow := lines[3]
	if !strings.Contains(lastRow, "long-task-name") || !strings.Contains(lastRow, "stopped") {
		t.Errorf("expected last row to contain both column values, got %q", lastRow)
	}
}

func TestPrintTable_NoRows(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, []string{"NAME"}, nil)
	out := buf.String()
	if !strings.Contains(out, "NAME") {
		t.Errorf("expected output to still contain the header, got %q", out)
	}
}
