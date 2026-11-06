package worker

import (
	"errors"
	"strings"
)

// ThumbnailRequested mirrors VideoThumbnailRequestedEvent in feedzmore-service.
type ThumbnailRequested struct {
	PostID             string `json:"postId"`
	MediaURL           string `json:"mediaUrl"`
	ThumbnailObjectKey string `json:"thumbnailObjectKey"`
}

// ThumbnailGenerated mirrors VideoThumbnailGeneratedEvent in feedzmore-service.
type ThumbnailGenerated struct {
	PostID             string `json:"postId"`
	ThumbnailObjectKey string `json:"thumbnailObjectKey"`
}

// ErrInvalid marks a message that can never succeed, so it must not be retried.
var ErrInvalid = errors.New("invalid message")

func (r ThumbnailRequested) validate() error {
	switch {
	case r.PostID == "":
		return errors.Join(ErrInvalid, errors.New("postId is empty"))
	case r.MediaURL == "" || r.ThumbnailObjectKey == "":
		return errors.Join(ErrInvalid, errors.New("mediaUrl and thumbnailObjectKey are required"))
	case strings.Contains(r.MediaURL, "..") || strings.Contains(r.ThumbnailObjectKey, ".."):
		return errors.Join(ErrInvalid, errors.New("object key must not contain '..'"))
	}
	return nil
}
