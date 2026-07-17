package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrObjectNotFound is returned when an object key does not exist. The media
// proxy treats it as a gone/unavailable result rather than a server error.
var ErrObjectNotFound = errors.New("object not found")

// ObjectStore abstracts private media storage. S3 endpoints and object keys
// never leave the backend: media is streamed through authenticated handlers.
type ObjectStore interface {
	Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	Delete(ctx context.Context, key string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Stat(ctx context.Context, key string) (int64, error)
	Health(ctx context.Context) error
}

type LocalStore struct {
	Root string
}

func NewLocalStore(root string) (*LocalStore, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &LocalStore{Root: root}, nil
}

func (s *LocalStore) path(key string) (string, error) {
	clean := filepath.Clean(key)
	if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", errors.New("invalid storage key")
	}
	return filepath.Join(s.Root, clean), nil
}

func (s *LocalStore) Put(_ context.Context, key string, body io.Reader, size int64, _ string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	written, err := io.Copy(file, io.LimitReader(body, size+1))
	if err != nil {
		return err
	}
	if written != size {
		return fmt.Errorf("stored %d bytes, expected %d", written, size)
	}
	return file.Sync()
}

func (s *LocalStore) Delete(_ context.Context, key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	return os.Open(path)
}

func (s *LocalStore) Stat(_ context.Context, key string) (int64, error) {
	path, err := s.path(key)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrObjectNotFound
		}
		return 0, err
	}
	return info.Size(), nil
}

func (s *LocalStore) Health(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		_, err := os.Stat(s.Root)
		return err
	}
}

type S3Store struct {
	client *minio.Client
	bucket string
}

func NewS3Store(endpoint, region, bucket, accessKey, secretKey string, pathStyle bool) (*S3Store, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("invalid S3 endpoint")
	}
	client, err := minio.New(parsed.Host, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: parsed.Scheme == "https", Region: region, BucketLookup: func() minio.BucketLookupType {
		if pathStyle {
			return minio.BucketLookupPath
		}
		return minio.BucketLookupAuto
	}()})
	if err != nil {
		return nil, err
	}
	return &S3Store{client: client, bucket: bucket}, nil
}

func (s *S3Store) EnsureBucket(ctx context.Context, region string) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region})
	}
	return nil
}

func (s *S3Store) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	// GetObject is lazy; probe the object so a missing key is reported now
	// rather than after headers have been committed to the client.
	if _, err := object.Stat(); err != nil {
		_ = object.Close()
		if isS3NotFound(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	return object, nil
}

func (s *S3Store) Stat(ctx context.Context, key string) (int64, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if isS3NotFound(err) {
			return 0, ErrObjectNotFound
		}
		return 0, err
	}
	return info.Size, nil
}

func (s *S3Store) Health(ctx context.Context) error {
	_, err := s.client.BucketExists(ctx, s.bucket)
	return err
}

// isS3NotFound reports whether an object-storage error indicates a missing key.
func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		return resp.Code == "NoSuchKey"
	}
	message := err.Error()
	return strings.Contains(message, "NoSuchKey") || strings.Contains(message, "404")
}
