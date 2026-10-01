package evidence

import "testing"

func TestParseMigrationSpecsRejectsDuplicateVersion(t *testing.T) {
	_, err := parseMigrationSpecs([]string{
		"001_evidence.sql",
		"002_projection.sql",
		"002_projection_retry.sql",
	})
	if err == nil {
		t.Fatal("versão duplicada de migration deveria falhar fechado")
	}
}
