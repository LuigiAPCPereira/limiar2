package terminal

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// ReadPasswordMasked reads a password from the given file descriptor while
// echoing one "*" per character typed. The terminal is restored to its
// original mode on return, even if the caller receives a signal or panics.
// Backspace deletes the last character (and its echo). Enter submits. Ctrl+C
// returns ErrInterrupted so the caller can surface a friendly message.
func ReadPasswordMasked(fd int) (string, error) {
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}

	restored := false
	restore := func() {
		if !restored {
			_ = term.Restore(fd, oldState)
			restored = true
		}
	}
	defer restore()

	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(b)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}
		switch b[0] {
		case '\r', '\n': // Enter
			// Restore terminal FIRST so the newline below is rendered in
			// cooked mode and doesn't drift on a quirky TTY driver.
			restore()
			fmt.Println()
			return string(buf), nil
		case 0x03: // Ctrl+C
			restore()
			fmt.Println("^C")
			return "", fmt.Errorf("interrompido pelo usuário")
		case 0x04: // Ctrl+D (EOF)
			restore()
			fmt.Println()
			return string(buf), nil
		case 0x7f, 0x08: // Backspace / Delete
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				// Erase the last '*' from the terminal: move back, space, back.
				fmt.Print("\b \b")
			}
		default:
			// Skip other control characters (arrows, etc.) so they don't pollute
			// the buffer; only printable characters become part of the password.
			if b[0] < 0x20 || b[0] > 0x7e {
				continue
			}
			buf = append(buf, b[0])
			fmt.Print("*")
		}
	}
}
