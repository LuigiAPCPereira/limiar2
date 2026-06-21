package logger

// NopLogger é uma implementação "no-op" (sem operação) de Logger que descarta todos os registros.
// É seguro para uso concorrente e útil como um padrão quando uma camada não
// precisa emitir logs (ex: testes, ou quando o chamador não forneceu um logger).
type NopLogger struct{}

// asserção em tempo de compilação de que NopLogger satisfaz Logger.
var _ Logger = NopLogger{}

func (NopLogger) Debug(_ string, _ ...any)      {}
func (NopLogger) Info(_ string, _ ...any)       {}
func (NopLogger) Warn(_ string, _ ...any)       {}
func (NopLogger) Error(_ string, _ ...any)      {}
func (NopLogger) With(_ ...any) Logger          { return NopLogger{} }
func (NopLogger) WithComponent(_ string) Logger { return NopLogger{} }

// IsInfoEnabled retorna sempre false: NopLogger descarta tudo.
func (NopLogger) IsInfoEnabled() bool { return false }

// NopPresenter é uma implementação "no-op" de Presenter que descarta todas as
// Mensagens_de_Apresentação. Útil em testes e no processor (que não tem saída interativa).
type NopPresenter struct{}

// asserção em tempo de compilação de que NopPresenter satisfaz Presenter.
var _ Presenter = NopPresenter{}

func (NopPresenter) Info(_ string)    {}
func (NopPresenter) Success(_ string) {}
func (NopPresenter) Warning(_ string) {}
func (NopPresenter) Error(_ string)   {}
func (NopPresenter) Step(_ string)    {}
