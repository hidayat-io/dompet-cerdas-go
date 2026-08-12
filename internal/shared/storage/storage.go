// Package storage uploads transaction attachments to Firebase Storage.
// Objects stay private — no makePublic, no signed URLs. The web app resolves a
// display URL from the stored path through the authenticated Storage SDK.
package storage

import (
	"context"
	"errors"
	"fmt"

	gcs "cloud.google.com/go/storage"
	fbstorage "firebase.google.com/go/v4/storage"
)

// Client writes objects into one Firebase Storage bucket.
type Client struct {
	bucket *gcs.BucketHandle
}

// New resolves the bucket handle. The bucket is not probed here; a missing
// bucket surfaces on the first upload, which the caller treats as non-fatal.
func New(ctx context.Context, fb *fbstorage.Client, bucketName string) (*Client, error) {
	if bucketName == "" {
		return nil, errors.New("storage: bucket name is required")
	}
	bucket, err := fb.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve bucket %q: %w", bucketName, err)
	}
	return &Client{bucket: bucket}, nil
}

// Upload writes one object. The writer must be closed even when Write fails,
// or the underlying stream leaks.
func (c *Client) Upload(ctx context.Context, path string, data []byte, contentType string) error {
	w := c.bucket.Object(path).NewWriter(ctx)
	w.ContentType = contentType

	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return fmt.Errorf("storage: write %q: %w", path, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("storage: close %q: %w", path, err)
	}
	return nil
}
