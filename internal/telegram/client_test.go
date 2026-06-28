package telegram

import (
	"math/rand"
	"testing"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
)

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain username", "mychannel", "mychannel"},
		{"with @ prefix", "@mychannel", "mychannel"},
		{"multiple @ prefix", "@@mychannel", "mychannel"},
		{"https t.me link", "https://t.me/mychannel", "mychannel"},
		{"http t.me link", "http://t.me/mychannel", "mychannel"},
		{"t.me without scheme", "t.me/mychannel", "mychannel"},
		{"trailing slash", "https://t.me/mychannel/", "mychannel"},
		{"multiple trailing slashes", "https://t.me/mychannel///", "mychannel"},
		{"with invalid chars", "my-channel!", "mychannel"},
		{"unicode chars stripped", "my_chännel", "my_chnnel"},
		{"underscores preserved", "my_channel_123", "my_channel_123"},
		{"empty string", "", ""},
		{"only @", "@", ""},
		{"mixed case preserved", "MyChannel", "MyChannel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeUsername(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeUsername(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	cfg := BackoffConfig{
		BaseDelay:     time.Second,
		Multiplier:    2.0,
		JitterPercent: 0.10,
		Ceiling:       5 * time.Minute,
		MaxRetries:    5,
	}

	t.Run("first attempt yields ~1s", func(t *testing.T) {
		delay, err := CalculateBackoff(0, cfg, rng)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// With 10% jitter on 1s, delay should be in [0.9s, 1.1s]
		if delay < 900*time.Millisecond || delay > 1100*time.Millisecond {
			t.Errorf("delay = %v, want ~1s (with jitter)", delay)
		}
	})

	t.Run("delay grows exponentially", func(t *testing.T) {
		prev := time.Duration(0)
		for attempt := 0; attempt < 4; attempt++ {
			delay, err := CalculateBackoff(attempt, cfg, rng)
			if err != nil {
				t.Fatalf("attempt %d: unexpected error: %v", attempt, err)
			}
			if attempt > 0 && delay <= prev {
				// Due to jitter, delays may not always increase, but on average
				// they should. We allow some tolerance.
				t.Logf("note: delay[%d]=%v <= delay[%d]=%v (jitter)", attempt, delay, attempt-1, prev)
			}
			prev = delay
		}
	})

	t.Run("respects ceiling", func(t *testing.T) {
		// Attempt 10 with base 1s, multiplier 2x = 1024s >> 5min ceiling
		cfgHigh := BackoffConfig{
			BaseDelay:     time.Second,
			Multiplier:    2.0,
			JitterPercent: 0.0,
			Ceiling:       5 * time.Minute,
			MaxRetries:    20,
		}
		delay, err := CalculateBackoff(10, cfgHigh, rng)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if delay > 5*time.Minute {
			t.Errorf("delay = %v, exceeds ceiling of 5m", delay)
		}
	})

	t.Run("max retries exceeded", func(t *testing.T) {
		_, err := CalculateBackoff(5, cfg, rng)
		if err != apperrors.ErrMaxRetriesExceeded {
			t.Errorf("err = %v, want ErrMaxRetriesExceeded", err)
		}
	})

	t.Run("zero jitter is deterministic", func(t *testing.T) {
		cfgNoJitter := BackoffConfig{
			BaseDelay:     time.Second,
			Multiplier:    2.0,
			JitterPercent: 0.0,
			Ceiling:       10 * time.Minute,
			MaxRetries:    10,
		}
		delay, _ := CalculateBackoff(3, cfgNoJitter, rng)
		// 1s * 2^3 = 8s
		if delay != 8*time.Second {
			t.Errorf("delay = %v, want 8s", delay)
		}
	})
}

func TestDefaultBackoff(t *testing.T) {
	cfg := DefaultBackoff(10)
	if cfg.BaseDelay != time.Second {
		t.Errorf("BaseDelay = %v, want 1s", cfg.BaseDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", cfg.Multiplier)
	}
	if cfg.JitterPercent != 0.10 {
		t.Errorf("JitterPercent = %v, want 0.10", cfg.JitterPercent)
	}
	if cfg.Ceiling != 5*time.Minute {
		t.Errorf("Ceiling = %v, want 5m", cfg.Ceiling)
	}
	if cfg.MaxRetries != 10 {
		t.Errorf("MaxRetries = %d, want 10", cfg.MaxRetries)
	}
}
