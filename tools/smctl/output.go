package smctl

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

func writeIndentedJSON(out io.Writer, value any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("encode response: %w", err)
	}
	return nil
}

// colorize applies attr only when w is a terminal.
// Non-file writers (and redirected files) stay plain so pipe/redirect/test output remains stable.
func colorize(w io.Writer, attr color.Attribute, s string) string {
	if !writerIsTTY(w) {
		return s
	}
	return color.New(attr).Sprint(s)
}

func writerIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
