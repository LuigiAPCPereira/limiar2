package storage_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/limiar/collector/internal/storage"
)

// openBenchRepo abre um DB Turso em disco (não em memória) para que o fsync
// seja mensurável — benchmarks em memória subestimariam o ganho do batch.
func openBenchRepo(b *testing.B) (*storage.DB, *storage.Repository, func()) {
	b.Helper()
	dir, err := os.MkdirTemp("", "limiar-bench-")
	if err != nil {
		b.Fatalf("mkdtemp: %v", err)
	}
	db, err := storage.Open(context.Background(), filepath.Join(dir, "bench.db"))
	if err != nil {
		_ = os.RemoveAll(dir)
		b.Fatalf("open: %v", err)
	}
	repo, err := storage.NewRepository(db.DB())
	if err != nil {
		_ = db.Close()
		_ = os.RemoveAll(dir)
		b.Fatalf("repo: %v", err)
	}
	// Insere canal para satisfazer a FK (channel_id) de raw_messages.
	if err := repo.AddChannel(context.Background(), &storage.Channel{
		ID: 1, Username: "bench", Title: "Bench", Active: true,
	}); err != nil {
		_ = repo.Close()
		_ = db.Close()
		_ = os.RemoveAll(dir)
		b.Fatalf("add_channel: %v", err)
	}
	return db, repo, func() {
		_ = repo.Close()
		_ = db.Close()
		_ = os.RemoveAll(dir)
	}
}

// makeBenchMessages produz N mensagens com payloads distintos (tamanho ~512B)
// para um único canal, espelhando o formato típico de update do Telegram.
func makeBenchMessages(n int, channelID int64) []*storage.RawMessage {
	msgs := make([]*storage.RawMessage, n)
	payload := []byte(`{"_":"updateNewMessage","message":{"_":"message","id":0,"peer_id":{"_":"peerChannel","channel_id":` +
		fmt.Sprintf("%d", channelID) + `},"message":"oferta relâmpago produto X https://t.me/canal","date":1700000000}}`)
	for i := 0; i < n; i++ {
		msgs[i] = &storage.RawMessage{
			ChannelID:     channelID,
			MessageID:     int64(i + 1),
			Payload:       payload,
			ReceivedAt:    time.Now().UTC(),
			SchemaVersion: 1,
		}
	}
	return msgs
}

// BenchmarkSaveRawMessage_Single mede o throughput baseline: 1 transação
// por mensagem, 1 fsync por transação.
func BenchmarkSaveRawMessage_Single(b *testing.B) {
	_, repo, cleanup := openBenchRepo(b)
	defer cleanup()
	msgs := makeBenchMessages(b.N, 1)

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := repo.SaveRawMessage(ctx, msgs[i]); err != nil {
			b.Fatalf("SaveRawMessage: %v", err)
		}
	}
}

// BenchmarkSaveRawMessageBatch_100 mede o throughput com lote de 100
// mensagens por transação. Espera-se redução de ~100 fsyncs para 1 fsync
// por lote, traduzindo em ganho de 10x–100x em disco rotacional/SSD.
// Mensagens são pré-alocadas fora do loop para que o B/op reflita apenas
// a operação de DB (mesma metodologia do benchmark Single).
func BenchmarkSaveRawMessageBatch_100(b *testing.B) {
	const batchSize = 100
	_, repo, cleanup := openBenchRepo(b)
	defer cleanup()

	msgs := makeBenchMessages(b.N*batchSize, 1)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		start := i * batchSize
		end := start + batchSize
		if _, _, err := repo.SaveRawMessageBatch(context.Background(), msgs[start:end]); err != nil {
			b.Fatalf("SaveRawMessageBatch: %v", err)
		}
	}
}
