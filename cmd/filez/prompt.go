package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/DasDarki/filez/internal/client/ui"
	"golang.org/x/term"
)

var stdin = bufio.NewReader(os.Stdin)

// ask prints a prompt and reads a trimmed line.
func ask(label string) string {
	fmt.Print(ui.Label.Render(label) + " ")
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

// askDefault reads a line, returning def when the input is empty.
func askDefault(label, def string) string {
	v := ask(fmt.Sprintf("%s %s", label, ui.Subtle.Render("["+def+"]")))
	if v == "" {
		return def
	}
	return v
}

// askSecret reads a line without echoing it (falls back to visible input if the
// terminal is not interactive).
func askSecret(label string) string {
	fmt.Print(ui.Label.Render(label) + " ")
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Println()
		if err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	line, _ := stdin.ReadString('\n')
	return strings.TrimSpace(line)
}

// confirm asks a yes/no question with a default.
func confirm(label string, def bool) bool {
	suffix := "[Y/n]"
	if !def {
		suffix = "[y/N]"
	}
	v := strings.ToLower(ask(label + " " + ui.Subtle.Render(suffix)))
	if v == "" {
		return def
	}
	return v == "y" || v == "yes" || v == "j" || v == "ja"
}

// info / ok / fail print styled status lines.
func info(msg string)   { fmt.Println(ui.Subtle.Render("• " + msg)) }
func okLine(msg string) { fmt.Println(ui.Success.Render("✓ ") + msg) }
func failLine(msg string) {
	fmt.Fprintln(os.Stderr, ui.Error.Render("✗ ")+msg)
}
