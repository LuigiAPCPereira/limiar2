package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWizardCreation verifica que o wizard pode ser instanciado
func TestWizardCreation(t *testing.T) {
	w := NewWizard()
	if w == nil {
		t.Fatal("NewWizard() returned nil")
	}
	if w.reader == nil {
		t.Fatal("wizard reader is nil")
	}
}

// TestWizardAppIDValidation verifica validação de App ID
func TestWizardAppIDValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty input", "", true},
		{"non-numeric", "abc", true},
		{"zero", "0", true},
		{"negative", "-123", true},
		{"valid", "12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStdin := os.Stdin
			defer func() { os.Stdin = oldStdin }()

			r, wFile, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			os.Stdin = r

			go func() {
				wFile.Write([]byte(tt.input + "\n"))
				wFile.Close()
			}()

			wizard := NewWizard()
			_, err = wizard.promptAppID()

			if (err != nil) != tt.wantErr {
				t.Errorf("promptAppID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSaveEnvFile verifica que o .env é criado com permissões 0600
func TestSaveEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	wizard := NewWizard()
	err := wizard.saveEnvFile(31620060, "test_hash_abc123")
	if err != nil {
		t.Fatalf("saveEnvFile failed: %v", err)
	}

	envPath := filepath.Join(tmpDir, ".env")
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("expected .env to exist: %v", err)
	}

	// Verify permissions are 0600
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("expected permissions 0600, got %o", mode)
	}

	// Verify content
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("failed to read .env: %v", err)
	}
	expectedContains := []string{
		"LIMIAR_APP_ID=31620060",
		"LIMIAR_API_HASH=test_hash_abc123",
	}
	for _, s := range expectedContains {
		if !indexOfBytes(content, s) {
			t.Errorf(".env missing line: %q\nGot:\n%s", s, content)
		}
	}
}

// indexOfBytes returns true if needle is contained in haystack.
func indexOfBytes(haystack []byte, needle string) bool {
	n := []byte(needle)
	if len(n) == 0 {
		return true
	}
	for i := 0; i+len(n) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(n); j++ {
			if haystack[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestReadPasswordMasked_TTYOnly verifica a leitura mascarada caractere a
// caractere. Requer TTY real; em ambiente de CI/pipe, faz skip com instrução
// de como testar manualmente (veja comentário).
func TestReadPasswordMasked_TTYOnly(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping TTY-dependent test in CI; run locally with: go test -run TestReadPasswordMasked")
	}
	t.Skip("requires interactive TTY; covered by manual end-to-end test (see wizard smoke test script)")
}
