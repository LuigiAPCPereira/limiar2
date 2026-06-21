package model

// BoolToInt converte bool para int (1/0) para persistência no SQLite/Turso.
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
