package config

import "testing"

func TestProcessorConfigCarriesURLResolverDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.ApplyDefaults()

	proc := cfg.ProcessorConfig()
	if proc.ResolveURLs {
		t.Fatal("ProcessorConfig().ResolveURLs = true, want false by default")
	}
	if proc.ResolveURLsLimit != 0 {
		t.Fatalf("ProcessorConfig().ResolveURLsLimit = %d, want 0", proc.ResolveURLsLimit)
	}
}

func TestProcessorConfigCarriesURLResolverOverrides(t *testing.T) {
	cfg := &Config{ResolveURLs: true, ResolveURLsLimit: 25}
	cfg.ApplyDefaults()

	proc := cfg.ProcessorConfig()
	if !proc.ResolveURLs {
		t.Fatal("ProcessorConfig().ResolveURLs = false, want true")
	}
	if proc.ResolveURLsLimit != 25 {
		t.Fatalf("ProcessorConfig().ResolveURLsLimit = %d, want 25", proc.ResolveURLsLimit)
	}
}
