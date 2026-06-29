package config_test

import (
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/limiar/collector/internal/config"
)

// valid retorna um Config que passa no Validate, como base para mutação nos testes.
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
		DashboardPort:        8080,
		PollInterval:         5 * time.Second,
		BatchSize:            50,
	}
}

func TestValidAcceptsBaseline(t *testing.T) {
	if err := valid().Validate(); err != nil {
		t.Fatalf("configuração base deve ser válida, obteve: %v", err)
	}
}

// Funcionalidade: limiar-collector, Propriedade 12: Config Defaults Applied (Padrões de Configuração Aplicados).
// Defaults() deve preencher os padrões documentados para campos opcionais.
func TestProperty12Defaults(t *testing.T) {
	c := &config.Config{}
	c.ApplyDefaults()

	if c.DBPath != "./limiar.db" {
		t.Errorf("DBPath padrão = %q, esperado ./limiar.db", c.DBPath)
	}
	if c.LogLevel != "info" {
		t.Errorf("LogLevel padrão = %q, esperado info", c.LogLevel)
	}
	if c.LogFormat != "" {
		t.Errorf("LogFormat padrão = %q, esperado vazio (resolvido por subcomando via ResolveFormat)", c.LogFormat)
	}
	if c.ShutdownTimeout != 15 {
		t.Errorf("ShutdownTimeout padrão = %d, esperado 15", c.ShutdownTimeout)
	}
	if c.MaxRetries != 10 {
		t.Errorf("MaxRetries padrão = %d, esperado 10", c.MaxRetries)
	}
	if c.IOTimeout != 30*time.Second {
		t.Errorf("IOTimeout padrão = %v, esperado 30s", c.IOTimeout)
	}
	if c.DispatcherBufferSize != 256 {
		t.Errorf("DispatcherBufferSize padrão = %d, esperado 256", c.DispatcherBufferSize)
	}
	if c.DBWriterBufferSize != 512 {
		t.Errorf("DBWriterBufferSize padrão = %d, esperado 512", c.DBWriterBufferSize)
	}
	if c.HistoryMax != 5000 {
		t.Errorf("HistoryMax padrão = %d, esperado 5000", c.HistoryMax)
	}
	if c.HistoryMaxDays != 30 {
		t.Errorf("HistoryMaxDays padrão = %d, esperado 30", c.HistoryMaxDays)
	}
}

// Funcionalidade: limiar-collector, Propriedade 11: Config Validation Collects All Errors (A Validação de Configuração Coleta Todos os Erros).
// Para N campos obrigatórios ausentes/inválidos, Validate deve referenciar todos os N.
func TestProperty11ValidateCollectsAllErrors(t *testing.T) {
	c := valid()
	c.AppID = 0       // inválido (obrigatório)
	c.APIHash = ""    // inválido (obrigatório)
	c.LogLevel = "xx" // enumeração inválida

	err := c.Validate()
	if err == nil {
		t.Fatal("esperava erro de validação agrupado, obteve nil")
	}
	msg := err.Error()
	for _, field := range []string{"app_id", "api_hash", "log_level"} {
		if !strings.Contains(msg, field) {
			t.Errorf("erro agrupado faltando %q; obteve: %s", field, msg)
		}
	}
}

func TestValidateMissingRequiredFields(t *testing.T) {
	c := valid()
	c.AppID = 0
	c.APIHash = ""
	err := c.Validate()
	if err == nil {
		t.Fatal("esperava erro por falta de AppID e APIHash")
	}
	if !strings.Contains(err.Error(), "app_id") || !strings.Contains(err.Error(), "api_hash") {
		t.Errorf("o erro deve nomear ambos os campos ausentes: %s", err.Error())
	}
}

// Funcionalidade: limiar-collector, Propriedade 13: Config Field Validation Rejects Invalid Values (Validação de Campo de Configuração Rejeita Valores Inválidos).
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
			t.Fatalf("esperava erro de validação para valor fora do intervalo, config=%+v", c)
		}
	})
}

func TestValidateAcceptsAllValidLogLevels(t *testing.T) {
	for _, lvl := range []string{"debug", "info", "warn", "error"} {
		c := valid()
		c.LogLevel = lvl
		if err := c.Validate(); err != nil {
			t.Errorf("LogLevel %q deveria ser válido: %v", lvl, err)
		}
	}
}

func TestValidateAcceptsAllValidLogFormats(t *testing.T) {
	for _, f := range []string{"json", "text", "pretty"} {
		c := valid()
		c.LogFormat = f
		if err := c.Validate(); err != nil {
			t.Errorf("LogFormat %q deveria ser válido: %v", f, err)
		}
	}
}

// Funcionalidade: limiar-collector, Propriedade 14: Sensitive Data Masking (Ocultação de Dados Sensíveis).
// Para qualquer APIHash, Config.String() não deve contê-lo literalmente.
func TestProperty14StringMasksAPIHash(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		hash := rapid.StringMatching(`[a-f0-9]{8,64}`).Draw(t, "api_hash")
		c := valid()
		c.APIHash = hash
		if strings.Contains(c.String(), hash) {
			t.Fatalf("Config.String() vazou APIHash %q: %s", hash, c.String())
		}
	})
}

func TestStringMasksButShowsOtherFields(t *testing.T) {
	c := valid()
	s := c.String()
	if !strings.Contains(s, "./limiar.db") {
		t.Errorf("String() deveria mostrar o DBPath (não sensível): %s", s)
	}
	if strings.Contains(s, c.APIHash) {
		t.Errorf("String() vazou APIHash: %s", s)
	}
}
