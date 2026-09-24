package terminal

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// ReadPasswordMasked lê uma senha do file descriptor fornecido enquanto
// ecoa um "*" para cada caractere digitado. O terminal é restaurado ao seu
// modo original no retorno, mesmo que o chamador receba um sinal ou cause panic.
// O Backspace exclui o último caractere (e seu eco). O Enter envia. Ctrl+C
// retorna um erro indicando interrupção para que o chamador possa exibir uma mensagem amigável.
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
			// Restaura o terminal PRIMEIRO para que a quebra de linha abaixo seja renderizada no
			// modo "cooked" e não cause desvios em drivers TTY peculiares.
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
				// Apaga o último '*' do terminal: move para trás, espaço, volta.
				fmt.Print("\b \b")
			}
		default:
			// Ignora outros caracteres de controle (setas, etc.) para que não poluam
			// o buffer; apenas caracteres imprimíveis se tornam parte da senha.
			if b[0] < 0x20 || b[0] > 0x7e {
				continue
			}
			buf = append(buf, b[0])
			fmt.Print("*")
		}
	}
}
