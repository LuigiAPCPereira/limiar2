package config_test

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/limiar/collector/internal/config"
)

// valid returns a Config that passes Validate, as a base for mutation in tests.
func valid() *config.Config {
	return &config.Config{
		AppID:                12345,
		APIHash:              "0123456789abcdef0123456789abcdef",
		DBPath:               "./limiar.db",
		LogLevel:             "info",
		LogFormat:            "json",
		ShutdownTimeout:      15,
		MaxRetries:           10,
		IOTimeout:            30 * time.Second,
		DispatcherBufferSize: 256,
		DBWriterBufferSize:   512,
		HistoryMax:           5000,
		HistoryMaxDays:       30,
	}
}

func TestValidAcceptsBaseline(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("baseline config should be valid, got: %v", err)
	}
}

// Feature: limiar-collector, Property 12: Config Defaults Applied.
// Defaults() must populate documented defaults for optional fields.
func TestProperty12Defaults(t *testing.T) {
	c := &config.Config{}
	c.ApplyDefaults()

	if c.DBPath != "./limiar.db" {
		t.Errorf("DBPath default = %q, want ./limiar.db", c.DBPath)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel default = %q, want info", c.LogLevel)
	}
	if c.LogFormat != "pretty" {
		t.Errorf("LogFormat default = %q, want pretty", c.LogFormat)
	}
	if c.ShutdownTimeout != 15 {
		t.Errorf("ShutdownTimeout default = %d, want 15", c.ShutdownTimeout)
	}
	if c.MaxRetries != 10 {
		t.Errorf("MaxRetries default = %d, want 10", c.MaxRetries)
	}
	if c.IOTimeout != 30*time.Second {
		t.Errorf("IOTimeout default = %v, want 30s", c.IOTimeout)
	}
	if c.DispatcherBufferSize != 256 {
		t.Errorf("DispatcherBufferSize default = %d, want 256", c.DispatcherBufferSize)
	}
	if c.DBWriterBufferSize != 512 {
		t.Errorf("DBWriterBufferSize default = %d, want 512", c.DBWriterBufferSize)
	}
	if c.HistoryMax != 5000 {
		t.Errorf("HistoryMax default = %d, want 5000", c.HistoryMax)
	}
	if c.HistoryMaxDays != 30 {
		t.Errorf("HistoryMaxDays default = %d, want 30", c.HistoryMaxDays)
	}
}

// Feature: limiar-collector, Property 11: Config Validation Collects All Errors.
// For N missing/invalid required fields, Validate must reference all N.
func TestProperty11ValidateCollectsAllErrors(t *testing.T) {
	c := valid()
	c.AppID = 0       // invalid (required)
	c.APIHash = ""    // invalid (required)
	c.LogLevel = "xx" // invalid enum

	err := c.Validate()
	if err == nil {
		t.Fatal("expected aggregated validation error, got nil")
	}
	msg := err.Error()
	for _, field := range []string{"app_id", "api_hash", "log_level"} {
		if !strings.Contains(msg, field) {
			t.Errorf("aggregated error missing %q; got: %s", field, msg)
		}
	}
}

func TestValidateMissingRequiredFields(t *testing.T) {
	c := valid()
	c.AppID = 0
	c.APIHash = ""
	err := c.Validate()
	if err == nil {
		t.Fatal("expected error for missing AppID and APIHash")
	}
	if !strings.Contains(err.Error(), "app_id") || !strings.Contains(err.Error(), "api_hash") {
		t.Errorf("error should name both missing fields: %s", err.Error())
	}
}

// Feature: limiar-collector, Property 13: Config Field Validation Rejects Invalid Values.
func TestProperty13RejectsInvalidValues(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		c := valid()

		switch rapid.IntRange(0, 4).Draw(t, "field") {
		case 0:
			c.LogLevel = rapid.StringMatching(`[a-z]{2,6}`).
				Filter(func(s string) bool {
					return s != "debug" && s != "info" && s != "warn" && s != "error"
				}).Draw(t, "bad_level")
		case 1:
			c.LogFormat = rapid.StringMatching(`[a-z]{2,6}`).
				Filter(func(s string) bool { return s != "json" && s != "text" && s != "pretty" }).
				Draw(t, "bad_format")
		case 2:
			c.ShutdownTimeout = rapid.OneOf(
				rapid.IntRange(-100, 0),
				rapid.IntRange(301, 10000),
			).Draw(t, "bad_shutdown")
		case 3:
			c.DispatcherBufferSize = rapid.OneOf(
				rapid.IntRange(-100, 63),
				rapid.IntRange(4097, 100000),
			).Draw(t, "bad_dispatcher")
		case 4:
			c.DBWriterBufferSize = rapid.OneOf(
				rapid.IntRange(-100, 127),
				rapid.IntRange(8193, 100000),
			).Draw(t, "bad_dbwriter")
		}

		if err := c.Validate(); err == nil {
			t.Fatalf("expected validation error for out-of-range value, config=%+v", c)
		}
	})
}

func TestValidateAcceptsAllValidLogLevels(t *testing.T) {
	for _, lvl := range []string{"debug", "info", "warn", "error"} {
		c := valid()
		c.LogLevel = lvl
		if err := c.Validate(); err != nil {
			t.Errorf("LogLevel %q should be valid: %v", lvl, err)
		}
	}
}

func TestValidateAcceptsAllValidLogFormats(t *testing.T) {
	for _, f := range []string{"json", "text", "pretty"} {
		c := valid()
		c.LogFormat = f
		if err := c.Validate(); err != nil {
			t.Errorf("LogFormat %q should be valid: %v", f, err)
		}
	}
}

// Feature: limiar-collector, Property 14: Sensitive Data Masking.
// For any APIHash, Config.String() must not contain it verbatim.
func TestProperty14StringMasksAPIHash(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hash := rapid.StringMatching(`[a-f0-9]{8,64}`).Draw(t, "api_hash")
		c := valid()
		c.APIHash = hash
		if strings.Contains(c.String(), hash) {
			t.Fatalf("Config.String() leaked APIHash %q: %s", hash, c.String())
		}
	})
}

func TestStringMasksButShowsOtherFields(t *testing.T) {
	c := valid()
	s := c.String()
	if !strings.Contains(s, "./limiar.db") {
		t.Errorf("String() should show non-sensitive DBPath: %s", s)
	}
	if strings.Contains(s, c.APIHash) {
		t.Errorf("String() leaked APIHash: %s", s)
	}
}
