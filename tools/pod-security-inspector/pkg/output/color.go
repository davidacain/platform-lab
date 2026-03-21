package output

import (
	"fmt"
	"io"
	"strings"
)

// ANSI color codes.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorGreen  = "\033[32m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

func red(s string) string    { return colorRed + s + colorReset }
func yellow(s string) string { return colorYellow + s + colorReset }
func green(s string) string  { return colorGreen + s + colorReset }
func bold(s string) string   { return colorBold + s + colorReset }
func dim(s string) string    { return colorDim + s + colorReset }

// cell is a single table cell with its visible text and an optional color.
type cell struct {
	text  string
	color func(string) string // nil means no color
}

func plain(text string) cell                    { return cell{text: text} }
func colored(text string, fn func(string) string) cell { return cell{text: text, color: fn} }

// renderTable prints a table with correct column alignment even when cells
// contain ANSI escape codes. Widths are computed from visible text only.
func renderTable(w io.Writer, headers []string, rows [][]cell) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if i < len(widths) && len(c.text) > widths[i] {
				widths[i] = len(c.text)
			}
		}
	}

	// Header row — bold, no color.
	var hparts []string
	for i, h := range headers {
		hparts = append(hparts, bold(fmt.Sprintf("%-*s", widths[i], h)))
	}
	fmt.Fprintln(w, strings.Join(hparts, "  "))

	// Data rows.
	for _, row := range rows {
		var parts []string
		for i, c := range row {
			padded := fmt.Sprintf("%-*s", widths[i], c.text)
			if c.color != nil {
				padded = c.color(padded)
			}
			parts = append(parts, padded)
		}
		fmt.Fprintln(w, strings.Join(parts, "  "))
	}
}

// renderIndentedTable is like renderTable but indents every line by a prefix.
func renderIndentedTable(w io.Writer, indent string, headers []string, rows [][]cell) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, c := range row {
			if i < len(widths) && len(c.text) > widths[i] {
				widths[i] = len(c.text)
			}
		}
	}

	var hparts []string
	for i, h := range headers {
		hparts = append(hparts, bold(fmt.Sprintf("%-*s", widths[i], h)))
	}
	fmt.Fprintln(w, indent+strings.Join(hparts, "  "))

	for _, row := range rows {
		var parts []string
		for i, c := range row {
			padded := fmt.Sprintf("%-*s", widths[i], c.text)
			if c.color != nil {
				padded = c.color(padded)
			}
			parts = append(parts, padded)
		}
		fmt.Fprintln(w, indent+strings.Join(parts, "  "))
	}
}
