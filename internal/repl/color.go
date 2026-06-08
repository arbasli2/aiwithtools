package repl

import (
	"os"
)

// ANSI escape codes used by the REPL. Kept here so the colour
// palette is easy to tune in one place.
const (
	AnsiReset  = "\x1b[0m"
	AnsiDim    = "\x1b[2m"
	AnsiBold   = "\x1b[1m"
	AnsiRed    = "\x1b[31m"
	AnsiGreen  = "\x1b[32m"
	AnsiYellow = "\x1b[33m"
	AnsiCyan   = "\x1b[36m"
)

// Colorize wraps s with the given ANSI code if colour output is
// enabled, otherwise returns s unchanged.
func Colorize(code, s string) string {
	if !colorEnabled {
		return s
	}
	return code + s + AnsiReset
}

// Convenience wrappers used across the package.
func cBold(s string) string   { return Colorize(AnsiBold, s) }
func cDim(s string) string    { return Colorize(AnsiDim, s) }
func cRed(s string) string    { return Colorize(AnsiRed, s) }
func cGreen(s string) string  { return Colorize(AnsiGreen, s) }
func cYellow(s string) string { return Colorize(AnsiYellow, s) }
func cCyan(s string) string   { return Colorize(AnsiCyan, s) }

// colorEnabled is set at package init time based on:
//   - NO_COLOR env var (https://no-color.org)
//   - Whether stdout looks like a terminal.
var colorEnabled = detectColor(os.Stdout)

func detectColor(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
