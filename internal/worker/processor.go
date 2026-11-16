package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/rizkysiregar/narrativ-media-worker/internal/storage"
	"github.com/rizkysiregar/narrativ-media-worker/internal/thumbnail"
)

type Processor struct {
	Store     storage.Store
	Extractor thumbnail.Extractor
}

// Process downloads the video, extracts a frame and uploads it to the requested key.
// The result is idempotent: a redelivery rewrites the same object with the same content.
func (p Processor) Process(ctx context.Context, req ThumbnailRequested) (ThumbnailGenerated, error) {
	if err := req.validate(); err != nil {
		return ThumbnailGenerated{}, err
	}

	dir, err := os.MkdirTemp("", "thumb-")
	if err != nil {
		return ThumbnailGenerated{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	videoPath := filepath.Join(dir, "video")
	thumbPath := filepath.Join(dir, "thumb.jpg")

	if err := p.Store.Download(ctx, req.MediaURL, videoPath); err != nil {
		return ThumbnailGenerated{}, fmt.Errorf("download %s: %w", req.MediaURL, err)
	}
	if err := p.Extractor.Extract(ctx, videoPath, thumbPath); err != nil {
		return ThumbnailGenerated{}, fmt.Errorf("extract thumbnail: %w", err)
	}
	if err := p.Store.Upload(ctx, req.ThumbnailObjectKey, thumbPath, "image/jpeg"); err != nil {
		return ThumbnailGenerated{}, fmt.Errorf("upload %s: %w", req.ThumbnailObjectKey, err)
	}

	slog.Info("thumbnail generated", "postId", req.PostID, "mediaId", req.MediaID, "key", req.ThumbnailObjectKey)
	return ThumbnailGenerated{PostID: req.PostID, MediaID: req.MediaID, ThumbnailObjectKey: req.ThumbnailObjectKey}, nil
}
