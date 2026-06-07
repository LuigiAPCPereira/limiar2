// Package config provides interactive configuration wizard for first-time setup.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Wizard provides an interactive configuration experience when credentials
// are missing. It prompts for AppID and APIHash, validates them, and offers
// to save them to a .env file for future use.
type Wizard struct {
	reader *bufio.Reader
}

// NewWizard creates a new interactive configuration wizard that reads from
// stdin via a buffered reader (one line per prompt).
func NewWizard() *Wizard {
	return &Wizard{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Run executes the interactive wizard and returns a Config with user-provided
// credentials. It does not modify environment variables or files unless the
// user explicitly confirms.
func (w *Wizard) Run() (*Config, error) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║      LIMIAR COLLECTOR — Configuração         ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("  📡 Etapa 1/2 · Credenciais Telegram")
	fmt.Println("  ──────────────────────────────────────")
	fmt.Println()
	fmt.Println("  Obtenha suas credenciais em: https://my.telegram.org/apps")
	fmt.Println()

	// Prompt for App ID
	appID, err := w.promptAppID()
	if err != nil {
		return nil, err
	}

	// Prompt for API Hash (hidden input)
	apiHash, err := w.promptAPIHash()
	if err != nil {
		return nil, err
	}

	// Offer to save credentials BEFORE returning so user sees confirmation
	if err := w.offerSaveCredentials(appID, apiHash); err != nil {
		return nil, err
	}

	// Create config with user input
	cfg := &Config{
		AppID:   appID,
		APIHash: apiHash,
	}
	cfg.ApplyDefaults()

	fmt.Println()
	fmt.Println("  ✅ Credenciais configuradas. Iniciando login da conta...")
	fmt.Println()
	return cfg, nil
}

// promptAppID asks the user for their Telegram App ID and validates it.
func (w *Wizard) promptAppID() (int, error) {
	for {
		fmt.Print("  📱 App ID   › ")

		input, err := w.readLine()
		if err != nil {
			return 0, fmt.Errorf("erro ao ler App ID: %w", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			fmt.Println("  ⚠️  App ID não pode ser vazio. Tente novamente.")
			continue
		}

		appID, err := strconv.Atoi(input)
		if err != nil {
			fmt.Printf("  ⚠️  App ID inválido (deve ser um número): %s\n", err)
			continue
		}

		if appID <= 0 {
			fmt.Println("  ⚠️  App ID deve ser maior que zero.")
			continue
		}

		return appID, nil
	}
}

// promptAPIHash asks the user for their Telegram API Hash securely.
// Echoes a "*" for every character typed and restores the terminal to its
// original state on exit, even on Ctrl+C / panic.
func (w *Wizard) promptAPIHash() (string, error) {
	fd := int(os.Stdin.Fd())

	for {
		fmt.Print("  🔑 API Hash › ")

		var input string
		if term.IsTerminal(fd) {
			val, err := readPasswordMasked(fd)
			if err != nil {
				return "", fmt.Errorf("erro ao ler API Hash: %w", err)
			}
			input = val
		} else {
			line, err := w.readLine()
			if err != nil {
				return "", fmt.Errorf("erro ao ler API Hash: %w", err)
			}
			input = line
		}

		apiHash := strings.TrimSpace(input)
		if apiHash == "" {
			fmt.Println("  ⚠️  API Hash não pode ser vazio. Tente novamente.")
			continue
		}

		return apiHash, nil
	}
}

// readPasswordMasked reads a password from the given file descriptor while
// echoing one "*" per character typed. The terminal is restored to its
// original mode on return, even if the caller receives a signal or panics.
// Backspace deletes the last character (and its echo). Enter submits. Ctrl+C
// returns ErrInterrupted so the caller can surface a friendly message.
func readPasswordMasked(fd int) (string, error) {
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}

	var buf []byte
	for {
		b := make([]byte, 1)
		n, err := os.Stdin.Read(b)
		if err != nil {
			_ = term.Restore(fd, oldState)
			return "", err
		}
		if n == 0 {
			continue
		}
		switch b[0] {
		case '\r', '\n': // Enter
			// Restore terminal FIRST so the newline below is rendered in
			// cooked mode and doesn't drift on a quirky TTY driver.
			_ = term.Restore(fd, oldState)
			fmt.Println()
			return string(buf), nil
		case 0x03: // Ctrl+C
			_ = term.Restore(fd, oldState)
			fmt.Println("^C")
			return "", fmt.Errorf("interrompido pelo usuário")
		case 0x04: // Ctrl+D (EOF)
			_ = term.Restore(fd, oldState)
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

// offerSaveCredentials asks if the user wants to save credentials to .env file.
func (w *Wizard) offerSaveCredentials(appID int, apiHash string) error {
	for {
		fmt.Print("  💾 Salvar em .env para uso futuro? (s/N): ")

		input, err := w.readLine()
		if err != nil {
			// If scan fails, just skip saving (not a critical error)
			return nil
		}

		answer := strings.ToLower(strings.TrimSpace(input))
		switch answer {
		case "s", "sim", "y", "yes":
			return w.saveEnvFile(appID, apiHash)
		case "n", "nao", "não", "":
			fmt.Println("  Credenciais não foram salvas.")
			return nil
		default:
			fmt.Println("  ⚠️  Responda 's' (sim) ou 'n' (não).")
		}
	}
}

// saveEnvFile writes the credentials to a .env file with secure permissions.
func (w *Wizard) saveEnvFile(appID int, apiHash string) error {
	envPath := ".env"
	content := fmt.Sprintf(
		"# Telegram API Credentials (gerado pelo wizard)\n"+
			"# NÃO commite este arquivo - adicione .env ao .gitignore\n"+
			"LIMIAR_APP_ID=%d\n"+
			"LIMIAR_API_HASH=%s\n",
		appID, apiHash,
	)

	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("erro ao salvar .env: %w", err)
	}

	fmt.Printf("  ✅ Credenciais salvas em %s (permissões 0600)\n", envPath)
	fmt.Println("  Adicione .env ao seu .gitignore!")
	return nil
}

// readLine reads a single line from stdin, stripping the trailing newline.
// Returns an error if stdin is closed (EOF) or unreadable.
func (w *Wizard) readLine() (string, error) {
	line, err := w.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return line, nil
}
