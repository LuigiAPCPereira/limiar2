// Package errors define o domínio de erros do limiar-collector: erros sentinel nomeados
// que os chamadores verificam com errors.Is, e um único helper Wrap que
// adiciona contexto de camada/operação enquanto preserva a identidade do erro encapsulado.
//
// As camadas devem encapsular todo erro que cruze um limite de pacote via Wrap, para
// que as mensagens sejam lidas como "layer: operation: cause" e a identidade do sentinel
// seja mantida através de aninhamentos arbitrários.
package errors

import (
	stderrors "errors"
	"fmt"
)

// Erros Sentinel para o domínio do collector. Os chamadores comparam contra estes
// com errors.Is em vez de fazer correspondência em strings de mensagens.
var (
	// ErrNotAuthenticated indica que nenhuma sessão válida do Telegram está persistida.
	ErrNotAuthenticated = stderrors.New("não autenticado")
	// ErrChannelNotFound indica que um canal solicitado está ausente do armazenamento.
	ErrChannelNotFound = stderrors.New("canal não encontrado")
	// ErrSessionCorrupted indica que uma sessão armazenada não pôde ser decodificada.
	ErrSessionCorrupted = stderrors.New("sessão corrompida")
	// ErrDBWriteFailed indica que uma gravação no banco de dados falhou após todas as tentativas.
	ErrDBWriteFailed = stderrors.New("falha ao gravar no banco de dados")
	// ErrMaxRetriesExceeded indica que um loop de repetição esgotou seu limite.
	ErrMaxRetriesExceeded = stderrors.New("máximo de tentativas excedido")
)

// Wrap anota err com a camada e a operação de origem, preservando o
// erro encapsulado para errors.Is/errors.As. Retorna nil quando err for nil, para que
// possa ser usado diretamente em declarações de retorno.
//
// A mensagem resultante tem o formato "camada: operação: causa".
func Wrap(layer, op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %s: %w", layer, op, err)
}
