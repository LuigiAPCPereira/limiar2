package model

import "time"

// DBTimeLayout é o formato de data/hora textual usado pelos padrões (defaults)
// datetime('now') do schema.
const DBTimeLayout = "2006-01-02 15:04:05"

// ParseDBTime converte uma string de data/hora do banco (formato "2006-01-02 15:04:05")
// para time.Time. Tenta fallback para RFC3339 se o formato primário falhar.
func ParseDBTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(DBTimeLayout, s)
	if err != nil {
		// Fallback para RFC3339 caso um chamador tenha armazenado dessa forma.
		if t2, err2 := time.Parse(time.RFC3339, s); err2 == nil {
			return t2
		}
		return time.Time{}
	}
	return t
}
