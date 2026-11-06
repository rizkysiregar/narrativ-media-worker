// Package thumbnail extracts a still frame from a video with ffmpeg.
package thumbnail

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Extractor interface {
	Extract(ctx context.Context, videoPath, outputPath string) error
}

type FFmpeg struct {
	Path string
}

// Extract grabs one JPEG frame. It seeks to 1s first; videos shorter than that
// yield no frame, so it falls back to the first frame.
func (f FFmpeg) Extract(ctx context.Context, videoPath, outputPath string) error {
	var lastErr error
	for _, seek := range []string{"1", "0"} {
		_ = os.Remove(outputPath)
		out, err := exec.CommandContext(ctx, f.Path, "-y",
			"-ss", seek,
			"-i", videoPath,
			"-frames:v", "1",
			"-update", "1",
			"-q:v", "2",
			outputPath,
		).CombinedOutput()
		if err == nil && hasContent(outputPath) {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		lastErr = fmt.Errorf("ffmpeg (-ss %s): %v: %s", seek, err, tail(string(out)))
	}
	return lastErr
}

func hasContent(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}

// tail keeps error messages short: ffmpeg prints a long banner before the real error.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 400 {
		return s[len(s)-400:]
	}
	return s
}
