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

// terminalAuthenticator implements gotd's auth.UserAuthenticator by prompting
// on an io.Writer and reading answers from an io.Reader (stdin/stdout in
// production). It supports the phone -> code -> optional 2FA flow. Sign-up is
// not supported: limiar-collector authenticates an existing userbot account.
type terminalAuthenticator struct {
	in       *bufio.Scanner
	out      io.Writer
	log      logger.Logger
	password string // optional pre-supplied 2FA password; prompted if empty
}

// compile-time assertion that we satisfy gotd's authenticator contract.
var _ auth.UserAuthenticator = (*terminalAuthenticator)(nil)

// newTerminalAuthenticator builds an interactive authenticator and prints the
// authentication section header so the user knows which step they are in.
func newTerminalAuthenticator(in io.Reader, out io.Writer, log logger.Logger) *terminalAuthenticator {
	printAuthSection(out)
	return &terminalAuthenticator{
		in:  bufio.NewScanner(in),
		out: out,
		log: log,
	}
}

// printAuthSection prints the "Login da conta" section header that visually
// connects this step to the config wizard that precedes it.
func printAuthSection(out io.Writer) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  📱 Etapa 2/2 · Login da conta Telegram")
	fmt.Fprintln(out, "  ──────────────────────────────────────")
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

// Phone prompts for the account phone number.
func (a *terminalAuthenticator) Phone(_ context.Context) (string, error) {
	fmt.Fprintln(a.out, "  Informe o número de telefone da conta (formato internacional).")
	return a.prompt("  📞 Telefone › ")
}

// Password prompts for the 2FA password. gotd calls this only when 2FA is
// required (it surfaces as the password step rather than ErrPasswordRequired
// to the caller of Flow.Run).
func (a *terminalAuthenticator) Password(_ context.Context) (string, error) {
	if a.password != "" {
		return a.password, nil
	}
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "  🔒 Verificação em duas etapas ativada.")
	return a.promptMasked("  🔐 Senha 2FA › ")
}

// Code prompts for the login code Telegram sends to the account.
func (a *terminalAuthenticator) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Fprintln(a.out)
	fmt.Fprintln(a.out, "  Telegram enviou um código para o seu aplicativo ou SMS.")
	return a.promptMasked("  🔢 Código › ")
}

// AcceptTermsOfService is unreachable for an existing account sign-in, but the
// interface requires it. We decline by signalling sign-up is unsupported.
func (a *terminalAuthenticator) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return &auth.SignUpRequired{}
}

// SignUp is not supported: limiar-collector never registers a new account.
func (a *terminalAuthenticator) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("telegram: cadastro de nova conta não é suportado")
}
