// Package ui renders terminal output.
package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/EMOBase/emobase-cli/internal/checks"
	"github.com/charmbracelet/lipgloss"
)

var color = true

// SetColor enables or disables ANSI styling.
func SetColor(enabled bool) {
	color = enabled
}

var (
	styleError = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	styleInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	styleMuted = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "250"})
	styleBold  = lipgloss.NewStyle().Bold(true)
)

func paint(s lipgloss.Style, v string) string {
	if !color {
		return v
	}
	return s.Render(v)
}

// Title renders a section heading.
func Title(s string) string { return paint(styleBold, s) }

// Muted renders secondary text.
func Muted(s string) string { return paint(styleMuted, s) }

// OK renders a success marker.
func OK(s string) string { return paint(styleOK, s) }

// Severity paints a finding severity label.
func Severity(s checks.Severity) string {
	switch s {
	case checks.Error:
		return paint(styleError, s.Label())
	case checks.Warn:
		return paint(styleWarn, s.Label())
	case checks.Info:
		return paint(styleInfo, s.Label())
	default:
		return paint(styleOK, s.Label())
	}
}

// Table renders a left-aligned table with a header separator.
func Table(headers []string, rows [][]string) string {
	cols := len(headers)
	widths := make([]int, cols)
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			if w := lipgloss.Width(row[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}
	var b strings.Builder
	writeRow := func(cells []string, paintHeader bool) {
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			pad := widths[i] - lipgloss.Width(cell)
			if pad < 0 {
				pad = 0
			}
			if paintHeader {
				b.WriteString(paint(styleBold, cell))
			} else {
				b.WriteString(cell)
			}
			if i < cols-1 {
				b.WriteString(strings.Repeat(" ", pad))
				b.WriteString("  ")
			}
		}
		b.WriteString("\n")
	}
	writeRow(headers, true)
	for i, w := range widths {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(paint(styleMuted, strings.Repeat("-", w)))
	}
	b.WriteString("\n")
	for _, row := range rows {
		writeRow(row, false)
	}
	return b.String()
}

// Findings prints a report as a table.
func Findings(w io.Writer, report *checks.Report) {
	var rows [][]string
	for _, f := range report.Findings {
		fix := f.Fix.String()
		if fix == "-" {
			fix = ""
		}
		rows = append(rows, []string{Severity(f.Severity), f.Scope, f.Message, fix})
	}
	fmt.Fprint(w, Table([]string{"", "scope", "finding", "fix"}, rows))
}

// Changes prints applied edits.
func Changes(w io.Writer, changes []checks.Change) {
	if len(changes) == 0 {
		fmt.Fprintln(w, "No changes were needed.")
		return
	}
	var rows [][]string
	for _, c := range changes {
		old, new := c.Old, c.New
		if c.Secret {
			old, new = "********", "********"
		}
		if old == "" {
			old = "(unset)"
		}
		rows = append(rows, []string{c.Scope, c.Key, old, new})
	}
	fmt.Fprint(w, Table([]string{"scope", "key", "old", "new"}, rows))
}
