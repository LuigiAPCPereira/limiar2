package errors_test

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	apperrors "github.com/limiar/collector/internal/errors"
)

// sentinels is the set of named domain errors the package must export.
var sentinels = []error{
	apperrors.ErrNotAuthenticated,
	apperrors.ErrChannelNotFound,
	apperrors.ErrSessionCorrupted,
	apperrors.ErrDBWriteFailed,
	apperrors.ErrMaxRetriesExceeded,
}

func TestWrapFormatsLayerAndOp(t *testing.T) {
	err := apperrors.Wrap("telegram", "connect", stderrors.New("connection refused"))
	got := err.Error()
	want := "telegram: connect: connection refused"
	if got != want {
		t.Fatalf("Wrap format = %q, want %q", got, want)
	}
}

func TestWrapPreservesSentinelOneLevel(t *testing.T) {
	err := apperrors.Wrap("telegram", "auth", apperrors.ErrNotAuthenticated)
	if !stderrors.Is(err, apperrors.ErrNotAuthenticated) {
		t.Fatalf("errors.Is lost sentinel after one Wrap")
	}
}

func TestWrapNilReturnsNil(t *testing.T) {
	if got := apperrors.Wrap("layer", "op", nil); got != nil {
		t.Fatalf("Wrap(_, _, nil) = %v, want nil", got)
	}
}

// Feature: limiar-collector, Property 15: Error Wrapping Preserves Sentinel Identity.
// For any sentinel wrapped N levels deep via Wrap(layer, op, sentinel),
// errors.Is(wrapped, sentinel) must return true.
func TestProperty15WrapPreservesSentinelNLevels(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		idx := rapid.IntRange(0, len(sentinels)-1).Draw(t, "sentinel")
		levels := rapid.IntRange(1, 12).Draw(t, "levels")
		sentinel := sentinels[idx]

		err := sentinel
		for i := 0; i < levels; i++ {
			layer := rapid.StringMatching(`[a-z]{1,8}`).Draw(t, fmt.Sprintf("layer%d", i))
			op := rapid.StringMatching(`[a-z_]{1,8}`).Draw(t, fmt.Sprintf("op%d", i))
			err = apperrors.Wrap(layer, op, err)
		}

		if !stderrors.Is(err, sentinel) {
			t.Fatalf("errors.Is lost sentinel after %d levels: %v", levels, err)
		}
	})
}

func TestSentinelsAreDistinct(t *testing.T) {
	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if stderrors.Is(sentinels[i], sentinels[j]) {
				t.Fatalf("sentinels %d and %d are not distinct", i, j)
			}
		}
	}
}

func TestSentinelMessagesNonEmpty(t *testing.T) {
	for _, s := range sentinels {
		if strings.TrimSpace(s.Error()) == "" {
			t.Fatalf("sentinel has empty message: %v", s)
		}
	}
}
