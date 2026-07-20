// Package config fornece um assistente de configuração interativo para a configuração inicial.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/limiar/collector/internal/terminal"
)

// Wizard fornece uma experiência de configuração interativa quando as credenciais
// estão ausentes. Ele solicita o AppID e o APIHash, os valida e oferece
// salvá-los em um arquivo .env para uso futuro.
type Wizard struct {
	reader *bufio.Reader
}

// NewWizard cria um novo assistente de configuração interativo que lê da
// entrada padrão (stdin) via um buffer (uma linha por prompt).
func NewWizard() *Wizard {
	return &Wizard{
		reader: bufio.NewReader(os.Stdin),
	}
}

// Run executa o assistente interativo e retorna um Config com as credenciais
// fornecidas pelo usuário. Ele não modifica variáveis de ambiente ou arquivos
// a menos que o usuário confirme explicitamente.
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

	// Prompt para App ID
	appID, err := w.promptAppID()
	if err != nil {
		return nil, err
	}

	// Prompt para API Hash (entrada oculta)
	apiHash, err := w.promptAPIHash()
	if err != nil {
		return nil, err
	}

	// Oferece para salvar as credenciais ANTES de retornar para que o usuário veja a confirmação
	if err := w.offerSaveCredentials(appID, apiHash); err != nil {
		return nil, err
	}

	// Cria a config com a entrada do usuário
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

// promptAppID pede ao usuário o seu App ID do Telegram e o valida.
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

// promptAPIHash pede ao usuário o seu API Hash do Telegram de forma segura.
// Ecoa um "*" para cada caractere digitado e restaura o terminal ao seu
// estado original na saída, mesmo em Ctrl+C / panic.
func (w *Wizard) promptAPIHash() (string, error) {
	fd := int(os.Stdin.Fd())

	for {
		fmt.Print("  🔑 API Hash › ")

		var input string
		if term.IsTerminal(fd) {
			val, err := terminal.ReadPasswordMasked(fd)
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

// offerSaveCredentials pergunta se o usuário deseja salvar as credenciais no arquivo .env.
func (w *Wizard) offerSaveCredentials(appID int, apiHash string) error {
	for {
		fmt.Print("  💾 Salvar em .env para uso futuro? (s/N): ")

		input, err := w.readLine()
		if err != nil {
			// Se a leitura falhar, apenas ignore o salvamento (não é um erro crítico)
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

// saveEnvFile escreve as credenciais em um arquivo .env com permissões seguras.
func (w *Wizard) saveEnvFile(appID int, apiHash string) error {
	envPath := ".env"
	content := fmt.Sprintf(
		"# Credenciais da API do Telegram (gerado pelo assistente)\n"+
			"# NÃO commite este arquivo - adicione .env ao .gitignore\n"+
			"LIMIAR_APP_ID=%d\n"+
			"LIMIAR_API_HASH=%s\n",
		appID, apiHash,
	)

	// Segurança: salva o .env restrito usando sintaxe octal moderna
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("erro ao salvar .env: %w", err)
	}

	fmt.Printf("  ✅ Credenciais salvas em %s (permissões 0o600)\n", envPath)
	fmt.Println("  Adicione .env ao seu .gitignore!")
	return nil
}

// readLine lê uma única linha do stdin, removendo a quebra de linha final.
// Retorna um erro se o stdin estiver fechado (EOF) ou ilegível.
func (w *Wizard) readLine() (string, error) {
	line, err := w.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return line, nil
}
