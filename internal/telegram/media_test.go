package telegram

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/model"
)

func TestExtractPhotoRequest_WithPhoto(t *testing.T) {
	fileRef := base64.StdEncoding.EncodeToString([]byte("test_ref"))
	payload := map[string]any{
		"ID":      float64(123),
		"Message": "test",
		"Media": map[string]any{
			"Photo": map[string]any{
				"ID":            float64(4985843018),
				"AccessHash":    float64(-7524180780),
				"FileReference": fileRef,
				"DCID":          float64(2),
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req, err := ExtractPhotoRequest(data)
	if err != nil {
		t.Fatalf("ExtractPhotoRequest: %v", err)
	}
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.PhotoID != 4985843018 {
		t.Errorf("PhotoID = %d, want 4985843018", req.PhotoID)
	}
	if req.AccessHash != -7524180780 {
		t.Errorf("AccessHash = %d, want -7524180780", req.AccessHash)
	}
	if req.DCID != 2 {
		t.Errorf("DCID = %d, want 2", req.DCID)
	}
	if string(req.FileReference) != "test_ref" {
		t.Errorf("FileReference = %q, want %q", string(req.FileReference), "test_ref")
	}
}

func TestExtractPhotoRequest_PreservesLargePhotoMetadataIDs(t *testing.T) {
	const (
		photoID    int64 = 4985843018296396847
		accessHash int64 = -7524180780117537531
	)
	fileRef := base64.StdEncoding.EncodeToString([]byte("test_ref"))
	payload := map[string]any{
		"ID":      int64(95232),
		"Message": "produto com foto",
		"Media": map[string]any{
			"Photo": map[string]any{
				"ID":            photoID,
				"AccessHash":    accessHash,
				"FileReference": fileRef,
				"DCID":          1,
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req, err := ExtractPhotoRequest(data)
	if err != nil {
		t.Fatalf("ExtractPhotoRequest: %v", err)
	}
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.PhotoID != photoID {
		t.Errorf("PhotoID = %d, want %d", req.PhotoID, photoID)
	}
	if req.AccessHash != accessHash {
		t.Errorf("AccessHash = %d, want %d", req.AccessHash, accessHash)
	}
}

func TestExtractPhotoRequest_NoMedia(t *testing.T) {
	payload := map[string]any{
		"ID":      float64(123),
		"Message": "no media",
	}
	data, _ := json.Marshal(payload)

	req, err := ExtractPhotoRequest(data)
	if err != nil {
		t.Fatalf("ExtractPhotoRequest: %v", err)
	}
	if req != nil {
		t.Errorf("expected nil request for message without photo, got %+v", req)
	}
}

func TestExtractPhotoRequest_MediaWithoutPhoto(t *testing.T) {
	payload := map[string]any{
		"ID": float64(123),
		"Media": map[string]any{
			"Video": map[string]any{"ID": float64(456)},
		},
	}
	data, _ := json.Marshal(payload)

	req, err := ExtractPhotoRequest(data)
	if err != nil {
		t.Fatalf("ExtractPhotoRequest: %v", err)
	}
	if req != nil {
		t.Errorf("expected nil for video media, got %+v", req)
	}
}

func TestExtractPhotoRequest_InvalidJSON(t *testing.T) {
	_, err := ExtractPhotoRequest([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestExtractPhotoRequest_NoFileReference(t *testing.T) {
	payload := map[string]any{
		"ID": float64(1),
		"Media": map[string]any{
			"Photo": map[string]any{
				"ID":         float64(100),
				"AccessHash": float64(200),
				"DCID":       float64(4),
			},
		},
	}
	data, _ := json.Marshal(payload)

	req, err := ExtractPhotoRequest(data)
	if err != nil {
		t.Fatalf("ExtractPhotoRequest: %v", err)
	}
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.PhotoID != 100 {
		t.Errorf("PhotoID = %d, want 100", req.PhotoID)
	}
	if req.FileReference != nil {
		t.Errorf("FileReference = %v, want nil", req.FileReference)
	}
}

func TestJSONToInt64(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want int64
	}{
		{"float64", float64(42), 42},
		{"negative float64", float64(-7524180780), -7524180780},
		{"string", "not a number", 0},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.JSONToInt64(tt.val)
			if got != tt.want {
				t.Errorf("JSONToInt64(%v) = %d, want %d", tt.val, got, tt.want)
			}
		})
	}
}

func TestJSONToInt(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want int
	}{
		{"float64", float64(4), 4},
		{"string", "abc", 0},
		{"nil", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.JSONToInt(tt.val)
			if got != tt.want {
				t.Errorf("JSONToInt(%v) = %d, want %d", tt.val, got, tt.want)
			}
		})
	}
}

func TestRunPhotoDownloadBatchUsesBoundedConcurrentDownloadsAndSerialHandler(t *testing.T) {
	ctx := context.Background()
	reqs := make([]media.PhotoDownloadRequest, 8)
	for i := range reqs {
		reqs[i] = media.PhotoDownloadRequest{PhotoID: int64(i + 1)}
	}

	var active int64
	var maxActive int64
	var handled int64
	var handlerActive int64
	err := runPhotoDownloadBatch(
		ctx,
		reqs,
		3,
		func(_ context.Context, req media.PhotoDownloadRequest) ([]byte, error) {
			current := atomic.AddInt64(&active, 1)
			for {
				maxSeen := atomic.LoadInt64(&maxActive)
				if current <= maxSeen || atomic.CompareAndSwapInt64(&maxActive, maxSeen, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt64(&active, -1)
			return []byte{byte(req.PhotoID)}, nil
		},
		func(photoID int64, data []byte, err error) {
			if current := atomic.AddInt64(&handlerActive, 1); current != 1 {
				t.Errorf("handler executou em paralelo: %d handlers ativos", current)
			}
			defer atomic.AddInt64(&handlerActive, -1)
			if err != nil {
				t.Errorf("download %d retornou erro: %v", photoID, err)
			}
			if len(data) != 1 || data[0] != byte(photoID) {
				t.Errorf("data para photo_id=%d = %v", photoID, data)
			}
			atomic.AddInt64(&handled, 1)
		},
	)
	if err != nil {
		t.Fatalf("runPhotoDownloadBatch: %v", err)
	}
	if got := atomic.LoadInt64(&handled); got != int64(len(reqs)) {
		t.Fatalf("handled = %d, want %d", got, len(reqs))
	}
	if got := atomic.LoadInt64(&maxActive); got < 2 || got > 3 {
		t.Fatalf("maxActive = %d, want between 2 and 3", got)
	}
}
