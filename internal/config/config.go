// Package config carrega e valida a configuração do limiar-collector a partir
// das variáveis de ambiente prefixadas com LIMIAR_ (e um arquivo de configuração opcional) via Viper.
//
// Load e Validate são deliberadamente separados: Load preenche a struct e
// aplica os padrões, enquanto Validate é chamado explicitamente antes de qualquer I/O e
// relata todos os campos inválidos de uma vez, em vez de falhar no primeiro.
package config

import (
	stderrors "errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/limiar/collector/internal/processor"
	"github.com/spf13/viper"
)

// Padrões (defaults) para campos de configuração opcionais.
const (
	defaultDBPath          = "./limiar.db"
	defaultLogLevel        = "info"
	defaultShutdownTimeout = 15
	defaultMaxRetries      = 10
	defaultIOTimeout       = 30 * time.Second
	defaultDispatcherBuf   = 256
	defaultDBWriterBuf     = 512
	defaultHistoryMax      = 5000
	defaultHistoryMaxDays  = 30
	defaultDashboardPort   = 8080
	defaultPollInterval    = 5 * time.Second
	defaultBatchSize       = 50
 )

// Limites de validação.
const (
	minShutdownTimeout = 1
	maxShutdownTimeout = 300
	minDispatcherBuf   = 64
	maxDispatcherBuf   = 4096
	minDBWriterBuf     = 128
	maxDBWriterBuf     = 8192
	minHistoryMax      = 100
	maxHistoryMax      = 100000
	minHistoryMaxDays  = 1
	maxHistoryMaxDays  = 365
)

// envPrefix é o prefixo para todas as variáveis de ambiente.
const envPrefix = "LIMIAR"

// Config contém todas as configurações do limiar-collector.
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
	HistoryMax           int           `mapstructure:"history_max"`
	HistoryMaxDays       int           `mapstructure:"history_max_days"`
	DashboardPort        int           `mapstructure:"dashboard_port"`

	// Processor
	PollInterval time.Duration `mapstructure:"processor_poll_interval"`
	BatchSize    int           `mapstructure:"processor_batch_size"`
 }

// Load lê as configurações das variáveis de ambiente com o prefixo LIMIAR_ e
// opcionalmente de um arquivo .env no diretório atual para dentro do Config e aplica
// os padrões. Se as credenciais obrigatórias (AppID, APIHash) estiverem ausentes E o stdin for
// um TTY, inicia um assistente interativo para coletá-las. Se o stdin não for um TTY
// (ex: CI/CD, scripts), retorna a configuração como está e o Validate() reportará
// os campos ausentes. Ele não valida; os chamadores devem chamar Validate
// explicitamente antes de realizar qualquer I/O.
func Load(v *viper.Viper) (*Config, error) {
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Tenta carregar o arquivo .env do diretório atual. É opcional e silenciosamente
	// ignorado se não estiver presente. Variáveis de ambiente têm precedência sobre o .env.
	if err := LoadDotEnv(); err != nil {
		return nil, fmt.Errorf("config: carregar .env: %w", err)
	}

	// Faz o bind das chaves explicitamente para que o AutomaticEnv resolva LIMIAR_<KEY> para cada uma.
	for _, key := range []string{
		"app_id", "api_hash", "db_path", "log_level", "log_format",
		"shutdown_timeout", "max_retries", "io_timeout",
		"dispatcher_buffer_size", "db_writer_buffer_size",
		"history_max", "history_max_days", "dashboard_port",
		"processor_poll_interval", "processor_batch_size",
	} {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("config: bind_env %s: %w", key, err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	// Se as credenciais estiverem ausentes e o stdin for um TTY, inicie o assistente interativo
	if (c.AppID == 0 || strings.TrimSpace(c.APIHash) == "") && isTerminal(os.Stdin) {
		wizardCfg, err := NewWizard().Run()
		if err != nil {
			return nil, fmt.Errorf("config: falha no assistente: %w", err)
		}
		c.AppID = wizardCfg.AppID
		c.APIHash = wizardCfg.APIHash
	}

	c.ApplyDefaults()
	return &c, nil
}

// LoadDotEnv lê os pares chave=valor com o prefixo LIMIAR_* de um arquivo .env no diretório
// atual e os define como variáveis de ambiente. Variáveis de ambiente
// já existentes têm precedência (não são sobrescritas). Linhas que começam com '#'
// e linhas em branco são ignoradas. Retorna nil se o .env não existir.
func LoadDotEnv() error {
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
		// Remove aspas opcionais que cercam o valor
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		// Não sobrescreve variáveis de ambiente existentes
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}

// isTerminal reporta se o file descriptor fornecido é um terminal.
// Usado para decidir se deve iniciar o assistente interativo.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ApplyDefaults preenche os campos opcionais não definidos com seus padrões documentados.
// Campos obrigatórios (AppID, APIHash) nunca recebem um valor padrão.
func (c *Config) ApplyDefaults() {
	if c.DBPath == "" {
		c.DBPath = defaultDBPath
	}
	if c.LogLevel == "" {
		c.LogLevel = defaultLogLevel
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
	if c.HistoryMax == 0 {
		c.HistoryMax = defaultHistoryMax
	}
	if c.HistoryMaxDays == 0 {
		c.HistoryMaxDays = defaultHistoryMaxDays
	}
	if c.DashboardPort == 0 {
		c.DashboardPort = defaultDashboardPort
	}
	if c.PollInterval == 0 {
		c.PollInterval = defaultPollInterval
	}
	if c.BatchSize == 0 {
		c.BatchSize = defaultBatchSize
	}
}

// Validate verifica todos os campos e retorna todos os problemas de validação
// agrupados num único erro, ao invés de parar no primeiro. Deve ser chamado antes
// de qualquer operação de E/S.
func (c *Config) Validate() error {
	var errs []error

	if c.AppID == 0 {
		errs = append(errs, stderrors.New("app_id é obrigatório"))
	}
	if strings.TrimSpace(c.APIHash) == "" {
		errs = append(errs, stderrors.New("api_hash é obrigatório"))
	}
	if !isValidLogLevel(c.LogLevel) {
		errs = append(errs, fmt.Errorf("log_level %q inválido (esperado: debug|info|warn|error)", c.LogLevel))
	}
	if c.LogFormat != "" && c.LogFormat != "json" && c.LogFormat != "text" && c.LogFormat != "pretty" {
		errs = append(errs, fmt.Errorf("log_format %q inválido (esperado: json|text|pretty)", c.LogFormat))
	}
	if c.ShutdownTimeout < minShutdownTimeout || c.ShutdownTimeout > maxShutdownTimeout {
		errs = append(errs, fmt.Errorf("shutdown_timeout %d fora do intervalo [%d,%d]", c.ShutdownTimeout, minShutdownTimeout, maxShutdownTimeout))
	}
	if c.DispatcherBufferSize < minDispatcherBuf || c.DispatcherBufferSize > maxDispatcherBuf {
		errs = append(errs, fmt.Errorf("dispatcher_buffer_size %d fora do intervalo [%d,%d]", c.DispatcherBufferSize, minDispatcherBuf, maxDispatcherBuf))
	}
	if c.DBWriterBufferSize < minDBWriterBuf || c.DBWriterBufferSize > maxDBWriterBuf {
		errs = append(errs, fmt.Errorf("db_writer_buffer_size %d fora do intervalo [%d,%d]", c.DBWriterBufferSize, minDBWriterBuf, maxDBWriterBuf))
	}
	if c.HistoryMax < minHistoryMax || c.HistoryMax > maxHistoryMax {
		errs = append(errs, fmt.Errorf("history_max %d fora do intervalo [%d,%d]", c.HistoryMax, minHistoryMax, maxHistoryMax))
	}
	if c.HistoryMaxDays < minHistoryMaxDays || c.HistoryMaxDays > maxHistoryMaxDays {
		errs = append(errs, fmt.Errorf("history_max_days %d fora do intervalo [%d,%d]", c.HistoryMaxDays, minHistoryMaxDays, maxHistoryMaxDays))
	}
	if c.PollInterval < time.Second || c.PollInterval > 5*time.Minute {
		errs = append(errs, fmt.Errorf("processor_poll_interval %s fora do intervalo [1s,5m]", c.PollInterval))
	}
	if c.BatchSize < 1 || c.BatchSize > 1000 {
		errs = append(errs, fmt.Errorf("processor_batch_size %d fora do intervalo [1,1000]", c.BatchSize))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config: validação falhou: %w", stderrors.Join(errs...))
	}
	return nil
}

// String implementa fmt.Stringer, mascarando o APIHash para que nunca apareça nos logs.
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{AppID:%d, APIHash:%s, DBPath:%s, LogLevel:%s, LogFormat:%s, "+
			"ShutdownTimeout:%d, MaxRetries:%d, IOTimeout:%s, "+
			"DispatcherBufferSize:%d, DBWriterBufferSize:%d, HistoryMax:%d, HistoryMaxDays:%d, "+
			"PollInterval:%s, BatchSize:%d}",
		c.AppID, maskSecret(c.APIHash), c.DBPath, c.LogLevel, c.LogFormat,
		c.ShutdownTimeout, c.MaxRetries, c.IOTimeout,
		c.DispatcherBufferSize, c.DBWriterBufferSize, c.HistoryMax, c.HistoryMaxDays,
		c.PollInterval, c.BatchSize,
	)
}

// ProcessorConfig retorna a configuração do processor derivada do Config unificado.
// Usado para criar processor.NewProcessor sem duplicar campos.
func (c *Config) ProcessorConfig() *processor.Config {
	return &processor.Config{
		DBPath:       c.DBPath,
		PollInterval: c.PollInterval,
		BatchSize:    c.BatchSize,
		LogLevel:     c.LogLevel,
		LogFormat:    c.LogFormat,
	}
}

func isValidLogLevel(level string) bool {
	switch level {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}

// maskSecret substitui um segredo por um marcador de ocultação fixo, nunca ecoando
// nenhuma porção do valor original.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	return "****"
}
