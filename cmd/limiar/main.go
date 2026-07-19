// Package main — binário unificado limiar.
//
// Este é o ponto de entrada de produção: compõe a árvore de comandos Cobra
// via cli.NewRootCmd e delega toda construção de dependências concretas ao
// Provider definido em provider.go.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/viper"

	"github.com/limiar/collector/internal/cli"
	"github.com/limiar/collector/internal/config"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "limiar:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(viper.New())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	p := newProvider(cfg)
	root := cli.NewRootCmd(p)
	return root.Execute()
}
