package logger

import (
	"bytes"
	"testing"
)

// dottedKeyBufferReference preserva a implementação anterior para comparação funcional e benchmark.
func dottedKeyBufferReference(groups []string, key string) string {
	if len(groups) == 0 {
		return key
	}
	var buf bytes.Buffer
	for _, group := range groups {
		buf.WriteString(group)
		buf.WriteByte('.')
	}
	buf.WriteString(key)
	return buf.String()
}

func TestDottedKeyMatchesBufferReference(t *testing.T) {
	cases := []struct {
		name   string
		groups []string
		key    string
	}{
		{name: "sem_grupos", key: "api_hash"},
		{name: "sem_grupos_e_chave_vazia"},
		{name: "um_grupo", groups: []string{"auth"}, key: "id"},
		{name: "grupos_aninhados", groups: []string{"auth", "telegram"}, key: "api_hash"},
		{name: "unicode", groups: []string{"configuração", "usuário"}, key: "ação"},
		{name: "componentes_vazios", groups: []string{"", "telegram"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := dottedKey(tc.groups, tc.key), dottedKeyBufferReference(tc.groups, tc.key); got != want {
				t.Fatalf("dottedKey(%q, %q) = %q; esperado %q", tc.groups, tc.key, got, want)
			}
		})
	}
}

var dottedKeyBenchmarkResult string

func BenchmarkDottedKey(b *testing.B) {
	groups := []string{"auth", "telegram"}
	key := "api_hash"
	b.Run("BufferReference", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			dottedKeyBenchmarkResult = dottedKeyBufferReference(groups, key)
		}
	})
	b.Run("Builder", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			dottedKeyBenchmarkResult = dottedKey(groups, key)
		}
	})
}
