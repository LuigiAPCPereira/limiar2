// Package config loads and validates limiar-collector configuration from
// LIMIAR_-prefixed environment variables (and an optional config file) via
// Viper.
//
// Load and Validate are deliberately separate: Load populates the struct and
// applies defaults, while Validate is called explicitly before any I/O and
// reports every invalid field at once rather than failing on the first.
package config

import (
	stderrors "errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Defaults for optional configuration fields.
const (
	defaultDBPath          = "./limiar.db"
	defaultLogLevel        = "info"
	defaultLogFormat       = "pretty"
	defaultShutdownTimeout = 15
	defaultMaxRetries      = 10
	defaultIOTimeout       = 30 * time.Second
	defaultDispatcherBuf   = 256
	defaultDBWriterBuf     = 512
)

// Validation bounds.
const (
	minShutdownTimeout = 1
	maxShutdownTimeout = 300
	minDispatcherBuf   = 64
	maxDispatcherBuf   = 4096
	minDBWriterBuf     = 128
	maxDBWriterBuf     = 8192
)

// envPrefix is the prefix for all environment variables.
const envPrefix = "LIMIAR"

// Config holds all limiar-collector settings.
type Config struct {
	AppID                int           `mapstructure:"app_id"`
	APIHash              string        `mapstructure:"api_hash"`
	DBPath               string        `mapstructure:"db_path"`
	LogLevel             string        `mapstructure:"log_level"`
	LogFormat            string        `mapstructure:"log_format"`
	ShutdownTimeout      int           `mapstructure:"shutdown_timeout"`
	MaxRetries           int           `mapstructure:"max_retries"`
	IOTimeout            time.Duration `mapstructure:"io_timeout"`
	DispatcherBufferSize int           `mapstructure:"dispatcher_buffer_size"`
	DBWriterBufferSize   int           `mapstructure:"db_writer_buffer_size"`
}

// Load reads configuration from LIMIAR_-prefixed environment variables and
// optionally a .env file in the current directory into a Config and applies
// defaults. If required credentials (AppID, APIHash) are missing AND stdin is
// a TTY, launches an interactive wizard to collect them. If stdin is not a TTY
// (e.g. CI/CD, scripts), returns the config as-is and Validate() will report
// the missing fields. It does not validate; callers must call Validate
// explicitly before performing any I/O.
func Load(v *viper.Viper) (*Config, error) {
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Try to load .env file from current directory. It is optional and silently
	// ignored if not present. Environment variables take precedence over .env.
	if err := loadDotEnv(); err != nil {
		return nil, fmt.Errorf("config: load .env: %w", err)
	}

	// Bind keys explicitly so AutomaticEnv resolves LIMIAR_<KEY> for each.
	for _, key := range []string{
		"app_id", "api_hash", "db_path", "log_level", "log_format",
		"shutdown_timeout", "max_retries", "io_timeout",
		"dispatcher_buffer_size", "db_writer_buffer_size",
	} {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("config: bind_env %s: %w", key, err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	// If credentials are missing and stdin is a TTY, launch interactive wizard
	if (c.AppID == 0 || strings.TrimSpace(c.APIHash) == "") && isTerminal(os.Stdin) {
		wizardCfg, err := NewWizard().Run()
		if err != nil {
			return nil, fmt.Errorf("config: wizard failed: %w", err)
		}
		c.AppID = wizardCfg.AppID
		c.APIHash = wizardCfg.APIHash
	}

	c.ApplyDefaults()
	return &c, nil
}

// loadDotEnv reads LIMIAR_* key=value pairs from a .env file in the current
// directory and sets them as environment variables. Existing environment
// variables take precedence (are not overwritten). Lines starting with '#'
// and blank lines are ignored. Returns nil if .env does not exist.
func loadDotEnv() error {
	data, err := os.ReadFile(".env")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip optional surrounding quotes
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		// Do not overwrite existing env vars
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

// isTerminal reports whether the given file descriptor is a terminal.
// Used to decide whether to launch the interactive wizard.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ApplyDefaults fills unset optional fields with their documented defaults.
// Required fields (AppID, APIHash) are never defaulted.
func (c *Config) ApplyDefaults() {
	if c.DBPath == "" {
		c.DBPath = defaultDBPath
	}
	if c.LogLevel == "" {
		c.LogLevel = defaultLogLevel
	}
	if c.LogFormat == "" {
		c.LogFormat = defaultLogFormat
	}
	if c.ShutdownTimeout == 0 {
		c.ShutdownTimeout = defaultShutdownTimeout
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultMaxRetries
	}
	if c.IOTimeout == 0 {
		c.IOTimeout = defaultIOTimeout
	}
	if c.DispatcherBufferSize == 0 {
		c.DispatcherBufferSize = defaultDispatcherBuf
	}
	if c.DBWriterBufferSize == 0 {
		c.DBWriterBufferSize = defaultDBWriterBuf
	}
}

// Validate checks every field and returns all validation problems joined into
// a single error, rather than stopping at the first. It must be called before
// any I/O.
func (c *Config) Validate() error {
	var errs []error

	if c.AppID == 0 {
		errs = append(errs, stderrors.New("app_id is required"))
	}
	if strings.TrimSpace(c.APIHash) == "" {
		errs = append(errs, stderrors.New("api_hash is required"))
	}
	if !isValidLogLevel(c.LogLevel) {
		errs = append(errs, fmt.Errorf("log_level %q invalid (want debug|info|warn|error)", c.LogLevel))
	}
	if c.LogFormat != "json" && c.LogFormat != "text" && c.LogFormat != "pretty" {
		errs = append(errs, fmt.Errorf("log_format %q invalid (want json|text|pretty)", c.LogFormat))
	}
	if c.ShutdownTimeout < minShutdownTimeout || c.ShutdownTimeout > maxShutdownTimeout {
		errs = append(errs, fmt.Errorf("shutdown_timeout %d out of range [%d,%d]", c.ShutdownTimeout, minShutdownTimeout, maxShutdownTimeout))
	}
	if c.DispatcherBufferSize < minDispatcherBuf || c.DispatcherBufferSize > maxDispatcherBuf {
		errs = append(errs, fmt.Errorf("dispatcher_buffer_size %d out of range [%d,%d]", c.DispatcherBufferSize, minDispatcherBuf, maxDispatcherBuf))
	}
	if c.DBWriterBufferSize < minDBWriterBuf || c.DBWriterBufferSize > maxDBWriterBuf {
		errs = append(errs, fmt.Errorf("db_writer_buffer_size %d out of range [%d,%d]", c.DBWriterBufferSize, minDBWriterBuf, maxDBWriterBuf))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config: validation failed: %w", stderrors.Join(errs...))
	}
	return nil
}

// String implements fmt.Stringer, masking APIHash so it never appears in logs.
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{AppID:%d, APIHash:%s, DBPath:%s, LogLevel:%s, LogFormat:%s, "+
			"ShutdownTimeout:%d, MaxRetries:%d, IOTimeout:%s, "+
			"DispatcherBufferSize:%d, DBWriterBufferSize:%d}",
		c.AppID, maskSecret(c.APIHash), c.DBPath, c.LogLevel, c.LogFormat,
		c.ShutdownTimeout, c.MaxRetries, c.IOTimeout,
		c.DispatcherBufferSize, c.DBWriterBufferSize,
	)
}

func isValidLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

// maskSecret replaces a secret with a fixed redaction marker, never echoing
// any portion of the original value.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	return "****"
}
