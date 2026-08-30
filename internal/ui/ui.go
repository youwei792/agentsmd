// Package ui provides terminal output helpers with ANSI color support that
// degrades gracefully: colors are disabled when stdout is not a TTY, when
// NO_COLOR is set, or when TERM is "dumb".
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

var enabled = detect()

func detect() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	if fi, err := os.Stdout.Stat(); err == nil {
		return fi.Mode()&os.ModeCharDevice != 0
	}
	return false
}

// SetEnabled overrides color detection (used by tests and --no-color).
func SetEnabled(v bool) { enabled = v }

func wrap(code, reset string, s string) string {
	if !enabled {
		return s
	}
	return code + s + reset
}

func Bold(s string) string     { return wrap("\x1b[1m", "\x1b[22m", s) }
func Dim(s string) string      { return wrap("\x1b[2m", "\x1b[22m", s) }
func Italic(s string) string   { return wrap("\x1b[3m", "\x1b[23m", s) }
func Red(s string) string      { return wrap("\x1b[31m", "\x1b[39m", s) }
func Green(s string) string    { return wrap("\x1b[32m", "\x1b[39m", s) }
func Yellow(s string) string   { return wrap("\x1b[33m", "\x1b[39m", s) }
func Magenta(s string) string  { return wrap("\x1b[35m", "\x1b[39m", s) }
func Cyan(s string) string     { return wrap("\x1b[36m", "\x1b[39m", s) }
func White(s string) string    { return wrap("\x1b[97m", "\x1b[39m", s) }
func BRed(s string) string     { return wrap("\x1b[1;31m", "\x1b[0m", s) }
func BGreen(s string) string   { return wrap("\x1b[1;32m", "\x1b[0m", s) }
func BYellow(s string) string  { return wrap("\x1b[1;33m", "\x1b[0m", s) }
func BCyan(s string) string    { return wrap("\x1b[1;36m", "\x1b[0m", s) }
func BMagenta(s string) string { return wrap("\x1b[1;35m", "\x1b[0m", s) }

// Section prints a bold header line preceded by a blank line (except first).
func Section(title string) {
	fmt.Println()
	fmt.Println(Bold(title))
}

func Item(format string, a ...any) {
	fmt.Printf("  %s\n", fmt.Sprintf(format, a...))
}

func Detail(format string, a ...any) {
	fmt.Printf("    %s\n", Dim(fmt.Sprintf(format, a...)))
}

// PrintJSON marshals v as indented JSON to stdout.
func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Pluralize returns "item"/"items" style strings.
func Pluralize(n int, singular string) string {
	if n == 1 {
		return singular
	}
	if strings.HasSuffix(singular, "s") {
		return singular + "es"
	}
	return singular + "s"
}
