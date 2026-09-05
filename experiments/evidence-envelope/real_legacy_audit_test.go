package evidenceenvelope

import (
	"os"
	"path/filepath"
	"testing"
)

const realLegacyCopyEnv = "LIMIAR_LEGACY_DB_COPY"

// TestRealLegacyCopyAudit executa o reconciliador do EXP-LIMIAR-006 contra uma
// cópia externa do banco legado. O teste é opt-in para impedir que CI ou um
// agente abra acidentalmente um banco operacional real.
//
// A saída deliberadamente não inclui payloads, caminho do arquivo, ids de
// mensagens/canais nem hashes por linha.
func TestRealLegacyCopyAudit(t *testing.T) {
	legacyPath := os.Getenv(realLegacyCopyEnv)
	if legacyPath == "" {
		t.Skip(realLegacyCopyEnv + " não definido; gate de cópia real não executado")
	}

	info, err := os.Stat(legacyPath)
	if err != nil {
		t.Fatalf("stat legacy copy: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("legacy copy deve ser arquivo regular")
	}

	before, err := fileSHA256(legacyPath)
	if err != nil {
		t.Fatalf("hash legacy copy before: %v", err)
	}

	source, err := openReadOnly(legacyPath)
	if err != nil {
		t.Fatalf("open legacy copy read-only: %v", err)
	}
	defer source.Close()

	var sourceIntegrity string
	if err := source.QueryRow(`PRAGMA integrity_check`).Scan(&sourceIntegrity); err != nil {
		t.Fatalf("legacy integrity_check: %v", err)
	}
	if sourceIntegrity != "ok" {
		t.Fatalf("legacy integrity_check=%q, want ok", sourceIntegrity)
	}

	var sourceRows int
	if err := source.QueryRow(`SELECT COUNT(*) FROM raw_messages`).Scan(&sourceRows); err != nil {
		t.Fatalf("count legacy raw_messages: %v", err)
	}

	targetPath := filepath.Join(t.TempDir(), "audit-target.db")
	target := openDB(t, targetPath)
	defer target.Close()
	if _, err := target.Exec(evidenceSchema + legacyImportLedgerSchema + evidenceAppendOnlyGuards); err != nil {
		t.Fatalf("create audit target schema: %v", err)
	}

	imported, err := importLegacyRawMessages(source, target, before)
	if err != nil {
		t.Fatalf("import legacy copy: %v", err)
	}
	if imported != sourceRows {
		t.Fatalf("imported=%d, source_rows=%d", imported, sourceRows)
	}

	retryImported, err := importLegacyRawMessages(source, target, before)
	if err != nil {
		t.Fatalf("retry legacy copy: %v", err)
	}
	if retryImported != 0 {
		t.Fatalf("retry imported=%d, want 0", retryImported)
	}

	var evidenceRows, ledgerRows int
	if err := target.QueryRow(`SELECT COUNT(*) FROM evidence`).Scan(&evidenceRows); err != nil {
		t.Fatalf("count evidence: %v", err)
	}
	if err := target.QueryRow(`SELECT COUNT(*) FROM legacy_import_ledger`).Scan(&ledgerRows); err != nil {
		t.Fatalf("count import ledger: %v", err)
	}
	if evidenceRows != sourceRows || ledgerRows != sourceRows {
		t.Fatalf("reconciliation counts source=%d evidence=%d ledger=%d", sourceRows, evidenceRows, ledgerRows)
	}

	var targetIntegrity string
	if err := target.QueryRow(`PRAGMA integrity_check`).Scan(&targetIntegrity); err != nil {
		t.Fatalf("target integrity_check: %v", err)
	}
	if targetIntegrity != "ok" {
		t.Fatalf("target integrity_check=%q, want ok", targetIntegrity)
	}

	after, err := fileSHA256(legacyPath)
	if err != nil {
		t.Fatalf("hash legacy copy after: %v", err)
	}
	if before != after {
		t.Fatal("legacy copy changed during read-only audit")
	}

	t.Logf("real legacy audit passed: rows=%d imported=%d retry_imported=%d source_integrity=ok target_integrity=ok", sourceRows, imported, retryImported)
}
