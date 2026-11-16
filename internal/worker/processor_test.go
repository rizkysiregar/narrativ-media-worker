package worker

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rizkysiregar/narrativ-media-worker/internal/config"
)

type fakeStore struct {
	downloaded   []string
	uploadedKey  string
	uploadedType string
	uploadedBody []byte
	downloadErr  error
}

func (f *fakeStore) Download(_ context.Context, key, path string) error {
	f.downloaded = append(f.downloaded, key)
	if f.downloadErr != nil {
		return f.downloadErr
	}
	return os.WriteFile(path, []byte("video"), 0o600)
}

func (f *fakeStore) Upload(_ context.Context, key, path, contentType string) error {
	f.uploadedKey, f.uploadedType = key, contentType
	body, err := os.ReadFile(path)
	f.uploadedBody = body
	return err
}

type fakeExtractor struct{ err error }

func (f fakeExtractor) Extract(_ context.Context, _, out string) error {
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(out, []byte("jpeg"), 0o600)
}

func validRequest() ThumbnailRequested {
	return ThumbnailRequested{
		PostID:             "post-1",
		MediaID:            "media-1",
		MediaURL:           "user/POST_FEED/video.mp4",
		ThumbnailObjectKey: "user/POST_FEED/video_thumb.jpg",
	}
}

func TestProcessUploadsThumbnailToRequestedKey(t *testing.T) {
	store := &fakeStore{}
	p := Processor{Store: store, Extractor: fakeExtractor{}}

	res, err := p.Process(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.PostID != "post-1" || res.MediaID != "media-1" || res.ThumbnailObjectKey != "user/POST_FEED/video_thumb.jpg" {
		t.Errorf("unexpected result: %+v", res)
	}
	if store.uploadedKey != "user/POST_FEED/video_thumb.jpg" || store.uploadedType != "image/jpeg" {
		t.Errorf("unexpected upload: key=%s type=%s", store.uploadedKey, store.uploadedType)
	}
	if string(store.uploadedBody) != "jpeg" {
		t.Errorf("uploaded body = %q", store.uploadedBody)
	}
}

func TestProcessRejectsInvalidRequest(t *testing.T) {
	cases := map[string]ThumbnailRequested{
		"no post id":      {MediaURL: "a.mp4", ThumbnailObjectKey: "a_thumb.jpg"},
		"no keys":         {PostID: "p"},
		"path traversal":  {PostID: "p", MediaURL: "../secret.mp4", ThumbnailObjectKey: "a_thumb.jpg"},
		"traversal (out)": {PostID: "p", MediaURL: "a.mp4", ThumbnailObjectKey: "../a_thumb.jpg"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{}
			_, err := Processor{Store: store, Extractor: fakeExtractor{}}.Process(context.Background(), req)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
			if len(store.downloaded) != 0 {
				t.Error("must not touch storage for an invalid request")
			}
		})
	}
}

func TestProcessReportsExtractorFailure(t *testing.T) {
	store := &fakeStore{}
	p := Processor{Store: store, Extractor: fakeExtractor{err: errors.New("boom")}}

	if _, err := p.Process(context.Background(), validRequest()); err == nil {
		t.Fatal("expected error")
	}
	if store.uploadedKey != "" {
		t.Error("must not upload when extraction failed")
	}
}

type countingHandler struct {
	calls   atomic.Int32
	failFor int32
	err     error
}

func (h *countingHandler) Process(_ context.Context, req ThumbnailRequested) (ThumbnailGenerated, error) {
	n := h.calls.Add(1)
	if n <= h.failFor {
		return ThumbnailGenerated{}, h.err
	}
	return ThumbnailGenerated{PostID: req.PostID, ThumbnailObjectKey: req.ThumbnailObjectKey}, nil
}

func newTestConsumer(h Handler, attempts int) *Consumer {
	c := NewConsumer(config.Config{MaxAttempts: attempts, ProcessTimeout: time.Second}, h)
	c.backoff = func(int) time.Duration { return 0 }
	return c
}

func TestRetriesTransientFailureThenSucceeds(t *testing.T) {
	h := &countingHandler{failFor: 2, err: errors.New("storage down")}

	res, err := newTestConsumer(h, 3).processWithRetry(context.Background(), validRequest())
	if err != nil || res.PostID != "post-1" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if h.calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", h.calls.Load())
	}
}

func TestGivesUpAfterMaxAttempts(t *testing.T) {
	h := &countingHandler{failFor: 99, err: errors.New("storage down")}

	if _, err := newTestConsumer(h, 3).processWithRetry(context.Background(), validRequest()); err == nil {
		t.Fatal("expected error")
	}
	if h.calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", h.calls.Load())
	}
}

func TestDoesNotRetryInvalidMessage(t *testing.T) {
	h := &countingHandler{failFor: 99, err: ErrInvalid}

	if _, err := newTestConsumer(h, 3).processWithRetry(context.Background(), validRequest()); err == nil {
		t.Fatal("expected error")
	}
	if h.calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", h.calls.Load())
	}
}
