package evidence

import (
	"context"
	"database/sql"
	"fmt"
)

// validateEvidenceSchema confirma que os objetos mínimos da migration de Evidence
// continuam presentes e vinculados à tabela esperada. A migration SQL permanece a
// autoridade do conteúdo desses objetos; esta verificação não executa reparos.
func validateEvidenceSchema(ctx context.Context, db *sql.DB) error {
	objects := []struct {
		name      string
		kind      string
		tableName string
	}{
		{name: "evidence", kind: "table", tableName: "evidence"},
		{name: "evidence_no_update", kind: "trigger", tableName: "evidence"},
		{name: "evidence_no_delete", kind: "trigger", tableName: "evidence"},
	}
	for _, object := range objects {
		var kind, tableName string
		err := db.QueryRowContext(ctx,
			`SELECT type, tbl_name FROM main.sqlite_schema WHERE name = ?`, object.name,
		).Scan(&kind, &tableName)
		if err == sql.ErrNoRows {
			return fmt.Errorf("schema de Evidence: objeto %q ausente", object.name)
		}
		if err != nil {
			return fmt.Errorf("schema de Evidence: inspecionar %q: %w", object.name, err)
		}
		if kind != object.kind || tableName != object.tableName {
			return fmt.Errorf("schema de Evidence: objeto %q type=%q tbl_name=%q, esperado type=%q tbl_name=%q",
				object.name, kind, tableName, object.kind, object.tableName)
		}
	}
	return nil
}
