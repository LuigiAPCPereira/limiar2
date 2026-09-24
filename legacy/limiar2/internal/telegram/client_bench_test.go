package telegram

import (
	"context"
	"testing"

	"github.com/gotd/td/tg"
)

func BenchmarkClientOnUpdate_Ignored(b *testing.B) {
	c := NewClient(1, "hash", 0, BackoffConfig{}, nil, nil, NewDispatcher(10, nil), nil)
	ctx := context.Background()

	// Update que não contém mensagem de canal (ex: UserStatus)
	u := &tg.UpdateShort{
		Update: &tg.UpdateUserStatus{
			UserID: 123,
			Status: &tg.UserStatusOnline{Expires: 123456},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.onUpdate(ctx, u)
	}
}
