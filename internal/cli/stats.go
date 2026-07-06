// Package cli — subcomando `stats`: relatório de distribuição de tipos e completude.
package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/storage"
)

// newStatsCmd constrói o subcomando `stats`: imprime distribuição de tipos,
// completude de campos, contagem por canal e período coberto. Read-only, one-shot.
func newStatsCmd(p Provider) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Relatório de distribuição de tipos e completude de campos",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg := p.Config()
			log := p.Logger()
			presenter := p.Presenter()

			db, err := storage.Open(ctx, cfg.DBPath, log.WithComponent("storage"))
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			collectorRepo, err := storage.NewRepository(db.DB())
			if err != nil {
				return fmt.Errorf("stats: criar collector repository: %w", err)
			}
			defer func() { _ = collectorRepo.Close() }()

			procRepo, err := storage.NewProcessorRepository(db.DB())
			if err != nil {
				return fmt.Errorf("stats: criar processor repository: %w", err)
			}
			defer func() { _ = procRepo.Close() }()

			out := cmd.OutOrStdout()

			// --- Raw Messages ---
			rawCount, err := collectorRepo.CountRawMessages(ctx)
			if err != nil {
				return err
			}

			// --- Processed Messages ---
			procCount, err := procRepo.CountProcessedMessages(ctx)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(out)
			presenter.Info("📊 Relatório Limiar")
			_, _ = fmt.Fprintln(out)

			// Volume
			_, _ = fmt.Fprintln(out, "  VOLUME")
			_, _ = fmt.Fprintln(out, "  "+strings.Repeat("─", 37))
			_, _ = fmt.Fprintf(out, "  Mensagens raw:         %d\n", rawCount)
			_, _ = fmt.Fprintf(out, "  Mensagens processadas: %d\n", procCount)
			if rawCount > 0 {
				pct := 100 * float64(procCount) / float64(rawCount)
				_, _ = fmt.Fprintf(out, "  Taxa de conversão:     %.1f%%\n", pct)
			}
			_, _ = fmt.Fprintln(out)

			// Distribuição por tipo
			typeStats, err := procRepo.CountProcessedByType(ctx)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(out, "  DISTRIBUIÇÃO POR TIPO")
			_, _ = fmt.Fprintln(out, "  "+strings.Repeat("─", 37))
			_, _ = fmt.Fprintf(out, "  %-20s %8s %7s\n", "TIPO", "QTD", "%")
			_, _ = fmt.Fprintln(out, "  "+strings.Repeat("─", 37))
			for _, ts := range typeStats {
				pct := 0.0
				if procCount > 0 {
					pct = 100 * float64(ts.Count) / float64(procCount)
				}
				_, _ = fmt.Fprintf(out, "  %-20s %8d %6.1f%%\n", ts.MessageType, ts.Count, pct)
			}
			_, _ = fmt.Fprintln(out, "  "+strings.Repeat("─", 37))
			_, _ = fmt.Fprintf(out, "  %-20s %8d\n", "TOTAL", procCount)
			_, _ = fmt.Fprintln(out)

			// Contagem por canal
			channelStats, err := collectorRepo.CountMessagesByChannel(ctx)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(out, "  MENSAGENS POR CANAL")
			_, _ = fmt.Fprintln(out, "  "+strings.Repeat("─", 40))
			_, _ = fmt.Fprintf(out, "  %-30s %8s\n", "CANAL", "MSGS")
			_, _ = fmt.Fprintln(out, "  "+strings.Repeat("─", 40))
			for _, cs := range channelStats {
				name := cs.Username
				if name == "" {
					name = fmt.Sprintf("id:%d", cs.ChannelID)
				}
				_, _ = fmt.Fprintf(out, "  %-30s %8d\n", name, cs.MessageCount)
			}
			_, _ = fmt.Fprintln(out)

			// Completude de campos (amostrados de deal_complete e deal_no_coupon)
			printCompleteness(ctx, out, procRepo)

			presenter.Success("Relatório concluído")
			return nil
		},
	}
}

// printCompleteness amostra mensagens processadas dos tipos deal e imprime
// a completude de cada campo relevante para o frontend.
func printCompleteness(ctx context.Context, out interface{ Write([]byte) (int, error) }, repo *storage.ProcessorRepository) {
	dealTypes := []string{"deal_complete", "deal_no_coupon"}

	_, _ = fmt.Fprintln(out, "  COMPLETUDE DE CAMPOS (tipos deal)")
	_, _ = fmt.Fprintln(out, "  "+strings.Repeat("─", 50))
	_, _ = fmt.Fprintf(out, "  %-20s %8s %8s %7s\n", "TIPO", "TOTAL", "CAMPO", "%")
	_, _ = fmt.Fprintln(out, "  "+strings.Repeat("─", 50))

	for _, msgType := range dealTypes {
		msgs, err := repo.ListProcessedMessages(ctx, 0, msgType, 1000, 0)
		if err != nil || len(msgs) == 0 {
			continue
		}

		total := len(msgs)
		var withPrice, withMerchant, withURL, withCoupon, withProductName int
		for _, m := range msgs {
			if m.HasPrice {
				withPrice++
			}
			if m.Merchant != "" {
				withMerchant++
			}
			if m.HasURL {
				withURL++
			}
			if m.HasCoupon {
				withCoupon++
			}
			if m.ProductName != "" {
				withProductName++
			}
		}

		fields := []struct {
			name  string
			count int
		}{
			{"preço", withPrice},
			{"merchant", withMerchant},
			{"url", withURL},
			{"cupom", withCoupon},
			{"product_name", withProductName},
		}

		for i, f := range fields {
			pct := 100 * float64(f.count) / float64(total)
			typeLabel := msgType
			totalLabel := fmt.Sprintf("%d", total)
			if i > 0 {
				typeLabel = ""
				totalLabel = ""
			}
			_, _ = fmt.Fprintf(out, "  %-20s %8s %-12s %5.1f%%\n", typeLabel, totalLabel, f.name, pct)
		}
		_, _ = fmt.Fprintln(out)
	}
}
