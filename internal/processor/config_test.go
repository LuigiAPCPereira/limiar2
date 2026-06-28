package processor

import (
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid defaults",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: 5 * time.Second,
				BatchSize:    50,
				LogLevel:     "info",
			},
			wantErr: false,
		},
		{
			name: "batch_size too small",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: 5 * time.Second,
				BatchSize:    0,
			},
			wantErr: true,
		},
		{
			name: "batch_size too large",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: 5 * time.Second,
				BatchSize:    1001,
			},
			wantErr: true,
		},
		{
			name: "poll_interval too short",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: 500 * time.Millisecond,
				BatchSize:    50,
			},
			wantErr: true,
		},
		{
			name: "poll_interval too long",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: 6 * time.Minute,
				BatchSize:    50,
			},
			wantErr: true,
		},
		{
			name: "boundary: batch_size=1",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: time.Second,
				BatchSize:    1,
			},
			wantErr: false,
		},
		{
			name: "boundary: batch_size=1000",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: 5 * time.Minute,
				BatchSize:    1000,
			},
			wantErr: false,
		},
		{
			name: "boundary: poll_interval=1s",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: time.Second,
				BatchSize:    50,
			},
			wantErr: false,
		},
		{
			name: "boundary: poll_interval=5m",
			cfg: Config{
				DBPath:       "./limiar.db",
				PollInterval: 5 * time.Minute,
				BatchSize:    50,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	t.Run("empty config gets defaults", func(t *testing.T) {
		c := &Config{}
		c.applyDefaults()

		if c.DBPath != defaultDBPath {
			t.Errorf("DBPath = %q, want %q", c.DBPath, defaultDBPath)
		}
		if c.PollInterval != defaultPollInterval {
			t.Errorf("PollInterval = %v, want %v", c.PollInterval, defaultPollInterval)
		}
		if c.BatchSize != defaultBatchSize {
			t.Errorf("BatchSize = %d, want %d", c.BatchSize, defaultBatchSize)
		}
		if c.LogLevel != defaultLogLevel {
			t.Errorf("LogLevel = %q, want %q", c.LogLevel, defaultLogLevel)
		}
	})

	t.Run("non-zero values preserved", func(t *testing.T) {
		c := &Config{
			DBPath:       "/custom/path.db",
			PollInterval: 10 * time.Second,
			BatchSize:    100,
			LogLevel:     "debug",
		}
		c.applyDefaults()

		if c.DBPath != "/custom/path.db" {
			t.Errorf("DBPath = %q, want /custom/path.db", c.DBPath)
		}
		if c.PollInterval != 10*time.Second {
			t.Errorf("PollInterval = %v, want 10s", c.PollInterval)
		}
		if c.BatchSize != 100 {
			t.Errorf("BatchSize = %d, want 100", c.BatchSize)
		}
		if c.LogLevel != "debug" {
			t.Errorf("LogLevel = %q, want debug", c.LogLevel)
		}
	})
}
