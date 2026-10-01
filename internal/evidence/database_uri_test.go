package evidence

import "testing"

func TestIsWindowsDriveLetter(t *testing.T) {
	tests := []struct {
		name   string
		volume string
		want   bool
	}{
		{name: "drive maiúsculo", volume: "C:", want: true},
		{name: "drive minúsculo", volume: "d:", want: true},
		{name: "vazio", volume: "", want: false},
		{name: "UNC", volume: `\\server\share`, want: false},
		{name: "um caractere", volume: "C", want: false},
		{name: "prefixo numérico", volume: "1:", want: false},
		{name: "prefixo de símbolo", volume: "@:", want: false},
		{name: "prefixo de ponto", volume: ".:", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWindowsDriveLetter(tt.volume); got != tt.want {
				t.Errorf("isWindowsDriveLetter(%q)=%v, esperado %v", tt.volume, got, tt.want)
			}
		})
	}
}
