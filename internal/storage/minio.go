// Package storage is the object-store access used by the worker.
package storage

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store interface {
	// Download saves the object to a local file.
	Download(ctx context.Context, key, path string) error
	// Upload stores the local file as the object.
	Upload(ctx context.Context, key, path, contentType string) error
}

type Minio struct {
	client *minio.Client
	bucket string
}

func NewMinio(endpoint string, secure bool, access, secret, bucket string) (*Minio, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &Minio{client: client, bucket: bucket}, nil
}

func (m *Minio) Download(ctx context.Context, key, path string) error {
	return m.client.FGetObject(ctx, m.bucket, key, path, minio.GetObjectOptions{})
}

func (m *Minio) Upload(ctx context.Context, key, path, contentType string) error {
	_, err := m.client.FPutObject(ctx, m.bucket, key, path, minio.PutObjectOptions{ContentType: contentType})
	return err
}
