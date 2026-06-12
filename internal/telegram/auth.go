package telegram

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/terminal"
)

// terminalAuthenticator implementa auth.UserAuthenticator do gotd solicitando
// entrada em um io.Writer e lendo as respostas de um io.Reader (stdin/stdout em
// produção). Ele suporta o fluxo telefone -> código -> senha 2FA opcional. O registro
// de nova conta (sign-up) não é suportado: limiar-collector autentica uma conta userbot existente.
type terminalAuthenticator struct {
	in       *bufio.Scanner
	out      io.Writer
	log      logger.Logger
	password string // senha 2FA opcional pré-fornecida; solicita se estiver vazia
}

// asserção em tempo de compilação de que satisfazemos o contrato de authenticator do gotd.
var _ auth.UserAuthenticator = (*terminalAuthenticator)(nil)

// newTerminalAuthenticator constrói um authenticator interativo e imprime o
// cabeçalho da seção de autenticação para que o usuário saiba em qual etapa está.
func newTerminalAuthenticator(in io.Reader, out io.Writer, log logger.Logger) *terminalAuthenticator {
	printAuthSection(out)
	return &terminalAuthenticator{
		in:  bufio.NewScanner(in),
		out: out,
		log: log,
	}
}

// printAuthSection imprime o cabeçalho da seção "Login da conta" que conecta visualmente
// esta etapa ao assistente de configuração (wizard) que a precede.
func printAuthSection(out io.Writer) {
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "  📱 Etapa 2/2 · Login da conta Telegram")
	_, _ = fmt.Fprintln(out, "  ──────────────────────────────────────")
}

func (a *terminalAuthenticator) prompt(label string) (string, error) {
	if _, err := fmt.Fprint(a.out, label); err != nil {
		return "", err
	}
	if !a.in.Scan() {
		if err := a.in.Err(); err != nil {
			return "", fmt.Errorf("leitura falhou: %w", err)
		}
		return "", fmt.Errorf("leitura falhou: EOF inesperado")
	}
	return strings.TrimSpace(a.in.Text()), nil
}

// promptMasked lê do terminal com echo mascarado (* por caractere) quando
// stdin é um TTY, caindo para leitura simples caso contrário.
func (a *terminalAuthenticator) promptMasked(label string) (string, error) {
	if _, err := fmt.Fprint(a.out, label); err != nil {
		return "", err
	}
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		val, err := terminal.ReadPasswordMasked(fd)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(val), nil
	}
	// Fallback para leitura de linha simples (pipe, CI)
	if !a.in.Scan() {
		if err := a.in.Err(); err != nil {
			return "", fmt.Errorf("leitura falhou: %w", err)
		}
		return "", fmt.Errorf("leitura falhou: EOF inesperado")
	}
	return strings.TrimSpace(a.in.Text()), nil
}

// Phone solicita o número de telefone da conta.
func (a *terminalAuthenticator) Phone(_ context.Context) (string, error) {
	_, _ = fmt.Fprintln(a.out, "  Informe o número de telefone da conta (formato internacional).")
	return a.prompt("  📞 Telefone › ")
}

// Password solicita a senha 2FA. O gotd chama isso apenas quando a 2FA é
// requerida (ele apresenta isso como a etapa da senha, em vez de ErrPasswordRequired
// para o chamador de Flow.Run).
func (a *terminalAuthenticator) Password(_ context.Context) (string, error) {
	if a.password != "" {
		return a.password, nil
	}
	_, _ = fmt.Fprintln(a.out)
	_, _ = fmt.Fprintln(a.out, "  🔒 Verificação em duas etapas ativada.")
	return a.promptMasked("  🔐 Senha 2FA › ")
}

// Code solicita o código de login que o Telegram envia para a conta.
func (a *terminalAuthenticator) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	_, _ = fmt.Fprintln(a.out)
	_, _ = fmt.Fprintln(a.out, "  Telegram enviou um código para o seu aplicativo ou SMS.")
	return a.promptMasked("  🔢 Código › ")
}

// AcceptTermsOfService é inacessível para o login de uma conta existente, mas a
// interface o exige. Recusamos sinalizando que o registro (sign-up) não é suportado.
func (a *terminalAuthenticator) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{}
}

// SignUp não é suportado: limiar-collector nunca registra uma nova conta.
func (a *terminalAuthenticator) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("telegram: cadastro de nova conta não é suportado")
}
