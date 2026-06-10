// Package logger define a interface Logger injetada em todas as camadas do
// limiar-collector e uma implementação padrão baseada no slog.
//
// A interface mantém os locais de chamada desacoplados do backend de log: trocar
// o slog pelo zerolog ou zap mais tarde requer alterar apenas este pacote. Nenhum outro
// pacote deve instanciar um logger concreto.
package logger

// Logger é o contrato de log injetado em cada camada via construtores.
// As implementações devem ser seguras para uso concorrente.
type Logger interface {
	// Debug faz o log em nível debug. args são pares de chave/valor alternados.
	Debug(msg string, args ...any)
	// Info faz o log em nível info. args são pares de chave/valor alternados.
	Info(msg string, args ...any)
	// Warn faz o log em nível warn. args são pares de chave/valor alternados.
	Warn(msg string, args ...any)
	// Error faz o log em nível error. args são pares de chave/valor alternados.
	Error(msg string, args ...any)
	// With retorna um Logger filho (child) que inclui os pares chave/valor fornecidos em
	// todos os registros subsequentes.
	With(args ...any) Logger
	// WithComponent retorna um Logger filho com o nome do componente vinculado.
	WithComponent(name string) Logger
}
