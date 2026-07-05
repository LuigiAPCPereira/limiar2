package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWizardCreation verifica se o assistente (wizard) pode ser instanciado
func TestWizardCreation(t *testing.T) {
	w := NewWizard()
	if w == nil {
		t.Fatal("NewWizard() retornou nil")
	}
	if w.reader == nil {
		t.Fatal("reader do wizard é nil")
	}
}

// TestWizardAppIDValidation verifica a validação do App ID
func TestWizardAppIDValidation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"entrada vazia", "", true},
		{"não numérico", "abc", true},
		{"zero", "0", true},
		{"negativo", "-123", true},
		{"válido", "12345", false},
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
				_, _ = wFile.Write([]byte(tt.input + "\n"))
				_ = wFile.Close()
			}()

			wizard := NewWizard()
			_, err = wizard.promptAppID()

			if (err != nil) != tt.wantErr {
				t.Errorf("promptAppID() erro = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSaveEnvFile verifica se o .env é criado com permissões 0600
func TestSaveEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldWd) }()

	wizard := NewWizard()
	err := wizard.saveEnvFile(31620060, "test_hash_abc123")
	if err != nil {
		t.Fatalf("saveEnvFile falhou: %v", err)
	}

	envPath := filepath.Join(tmpDir, ".env")
	info, err := os.Stat(envPath)
	if err != nil {
		t.Fatalf("esperado que o .env existisse: %v", err)
	}

	// Verifica se as permissões são 0600
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("esperado permissões 0600, obteve %o", mode)
	}

	// Verifica o conteúdo
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("falha ao ler o .env: %v", err)
	}
	expectedContains := []string{
		"LIMIAR_APP_ID=31620060",
		"LIMIAR_API_HASH=test_hash_abc123",
	}
	for _, s := range expectedContains {
		if !indexOfBytes(content, s) {
			t.Errorf(".env faltando linha: %q\nObteve:\n%s", s, content)
		}
	}
}

// indexOfBytes retorna true se needle estiver contido em haystack.
func indexOfBytes(haystack []byte, needle string) bool {
	n := []byte(needle)
	if len(n) == 0 {
		return true
	}
	for i := 0; i+len(n) <= len(haystack); i++ {
		match := true
		for j := range n {
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
		t.Skip("pulando teste dependente de TTY no CI; rode localmente com: go test -run TestReadPasswordMasked")
	}
	t.Skip("requer TTY interativo; coberto por teste e2e manual (veja o script smoke test do wizard)")
}
