package processor

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Defaults do processor.
const (
	defaultPollInterval = 5 * time.Second
	defaultBatchSize    = 50
	defaultDBPath       = "./limiar.db"
	defaultLogLevel     = "info"
)

// Config do limiar-processor. Sem credenciais Telegram — o processor
// não interage com MTProto, apenas lê do banco compartilhado.
type Config struct {
	DBPath       string        `mapstructure:"db_path"`
	PollInterval time.Duration `mapstructure:"processor_poll_interval"`
	BatchSize    int           `mapstructure:"processor_batch_size"`
	LogLevel     string        `mapstructure:"log_level"`
	LogFormat    string        `mapstructure:"log_format"`
}

// LoadConfig lê configuração de variáveis LIMIAR_ e aplica defaults.
func LoadConfig(v *viper.Viper) (*Config, error) {
	v.SetEnvPrefix("LIMIAR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := loadDotEnv(); err != nil {
		return nil, fmt.Errorf("processor: load .env: %w", err)
	}

	for _, key := range []string{
		"db_path", "processor_poll_interval", "processor_batch_size",
		"log_level", "log_format",
	} {
		if err := v.BindEnv(key); err != nil {
			return nil, fmt.Errorf("processor: bind_env %s: %w", key, err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("processor: unmarshal config: %w", err)
	}

	c.applyDefaults()
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.DBPath == "" {
		c.DBPath = defaultDBPath
	}
	if c.PollInterval == 0 {
		c.PollInterval = defaultPollInterval
	}
	if c.BatchSize == 0 {
		c.BatchSize = defaultBatchSize
	}
	if c.LogLevel == "" {
		c.LogLevel = defaultLogLevel
	}
}

// Validate verifica constraints do config.
func (c *Config) Validate() error {
	if c.BatchSize < 1 || c.BatchSize > 1000 {
		return fmt.Errorf("processor: batch_size %d fora do intervalo [1,1000]", c.BatchSize)
	}
	if c.PollInterval < time.Second || c.PollInterval > 5*time.Minute {
		return fmt.Errorf("processor: poll_interval %s fora do intervalo [1s,5m]", c.PollInterval)
	}
	return nil
}

func loadDotEnv() error {
	data, err := os.ReadFile(".env")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
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
		if len(val) >= 2 {
			first, last := val[0], val[len(val)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
	return nil
}
