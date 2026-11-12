package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// readPromptLine reads one line without buffering ahead. The repository
// init.sh scripts inherit the same stdin after the CLI prompt, so consuming
// bytes beyond the newline would make their prompts miss the input.
func readPromptLine(r io.Reader) (string, error) {
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			switch buf[0] {
			case '\n':
				return b.String(), nil
			case '\r':
			default:
				b.WriteByte(buf[0])
			}
		}
		if err != nil {
			if err == io.EOF && b.Len() > 0 {
				return b.String(), nil
			}
			return b.String(), err
		}
	}
}

// canPrompt reports whether stdin is a terminal.
func canPrompt(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(f.Fd())
}

// prompt asks a question with a default value. The answer is written to value.
func prompt(cmd *cobra.Command, label, def string, value *string) error {
	if *value != "" {
		return nil
	}
	if !canPrompt(cmd.InOrStdin()) {
		return fmt.Errorf("cannot ask %q: stdin is not a terminal (pass the matching flag or --yes)", label)
	}
	out := cmd.ErrOrStderr()
	if def == "" {
		fmt.Fprintf(out, "%s: ", label)
	} else {
		fmt.Fprintf(out, "%s [%s]: ", label, def)
	}
	line, err := readPromptLine(cmd.InOrStdin())
	if err != nil && line == "" {
		return fmt.Errorf("reading %q: %w", label, err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		line = def
	}
	*value = line
	return nil
}

// promptEdit always asks, using current as the default. It is used for forms
// where the existing value should be shown and pressing Enter keeps it.
func promptEdit(cmd *cobra.Command, label, current string) (string, error) {
	if !canPrompt(cmd.InOrStdin()) {
		return "", fmt.Errorf("cannot ask %q: stdin is not a terminal", label)
	}
	out := cmd.ErrOrStderr()
	if current == "" {
		fmt.Fprintf(out, "%s: ", label)
	} else {
		fmt.Fprintf(out, "%s [%s]: ", label, current)
	}
	line, err := readPromptLine(cmd.InOrStdin())
	if err != nil && line == "" {
		return "", fmt.Errorf("reading %q: %w", label, err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return current, nil
	}
	return line, nil
}

// promptRequired asks until the user enters a non-empty value. It has no
// default; the example is shown only as a hint.
func promptRequired(cmd *cobra.Command, label, example string) (string, error) {
	if !canPrompt(cmd.InOrStdin()) {
		return "", fmt.Errorf("cannot ask %q: stdin is not a terminal (pass the matching flag)", label)
	}
	out := cmd.ErrOrStderr()
	for {
		if example == "" {
			fmt.Fprintf(out, "%s: ", label)
		} else {
			fmt.Fprintf(out, "%s (for example %s): ", label, example)
		}
		line, err := readPromptLine(cmd.InOrStdin())
		if err != nil && line == "" {
			return "", fmt.Errorf("reading %q: %w", label, err)
		}
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
		fmt.Fprintln(out, "  This value is required.")
	}
}

// promptConfirm asks a yes/no question.
func promptConfirm(cmd *cobra.Command, label string, def bool) (bool, error) {
	if !canPrompt(cmd.InOrStdin()) {
		return def, fmt.Errorf("cannot ask %q: stdin is not a terminal", label)
	}
	out := cmd.ErrOrStderr()
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	for {
		fmt.Fprintf(out, "%s %s: ", label, hint)
		line, err := readPromptLine(cmd.InOrStdin())
		if err != nil && line == "" {
			return def, fmt.Errorf("reading %q: %w", label, err)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, "  Enter y or n.")
		}
	}
}

// promptChoice asks the user to choose one numbered item.
func promptChoice(cmd *cobra.Command, label string, items []string, def int) (int, error) {
	if !canPrompt(cmd.InOrStdin()) {
		return 0, fmt.Errorf("cannot ask %q: stdin is not a terminal", label)
	}
	out := cmd.ErrOrStderr()
	for {
		fmt.Fprintln(out, label)
		for i, item := range items {
			fmt.Fprintf(out, "  %d) %s\n", i+1, item)
		}
		fmt.Fprintf(out, "Choose [%d]: ", def)
		line, err := readPromptLine(cmd.InOrStdin())
		if err != nil && line == "" {
			return 0, fmt.Errorf("reading %q: %w", label, err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def, nil
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= 1 && n <= len(items) {
			return n, nil
		}
		fmt.Fprintf(out, "  Enter a number between 1 and %d.\n", len(items))
	}
}
