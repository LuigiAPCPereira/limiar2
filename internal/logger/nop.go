package logger

// NopLogger is a no-op Logger implementation that discards every record.
// It is safe for concurrent use and useful as a default when a layer does not
// need to emit logs (e.g. tests, or when the caller did not provide a logger).
type NopLogger struct{}

func (NopLogger) Debug(_ string, _ ...any)      {}
func (NopLogger) Info(_ string, _ ...any)       {}
func (NopLogger) Warn(_ string, _ ...any)       {}
func (NopLogger) Error(_ string, _ ...any)      {}
func (NopLogger) With(_ ...any) Logger          { return NopLogger{} }
func (NopLogger) WithComponent(_ string) Logger { return NopLogger{} }
