package logger

// NopLogger é uma implementação "no-op" (sem operação) de Logger que descarta todos os registros.
// É seguro para uso concorrente e útil como um padrão quando uma camada não
// precisa emitir logs (ex: testes, ou quando o chamador não forneceu um logger).
type NopLogger struct{}

func (NopLogger) Debug(_ string, _ ...any)      {}
func (NopLogger) Info(_ string, _ ...any)       {}
func (NopLogger) Warn(_ string, _ ...any)       {}
func (NopLogger) Error(_ string, _ ...any)      {}
func (NopLogger) With(_ ...any) Logger          { return NopLogger{} }
func (NopLogger) WithComponent(_ string) Logger { return NopLogger{} }
