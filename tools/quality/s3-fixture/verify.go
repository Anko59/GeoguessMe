package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"time"

	"github.com/minio/minio-go/v7"
)

type manifestObject struct {
	Key          string            `json:"key"`
	Size         int64             `json:"size"`
	SHA256       string            `json:"sha256"`
	ContentType  string            `json:"content_type"`
	Headers      map[string]string `json:"headers"`
	UserMetadata map[string]string `json:"user_metadata"`
}

func bucketManifest(ctx context.Context, client *minio.Client, bucket string) ([]manifestObject, error) {
	result := make([]manifestObject, 0)
	for listed := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if listed.Err != nil {
			return nil, listed.Err
		}
		if len(result) >= 1000000 {
			return nil, errors.New("local fixture manifest exceeds one million objects")
		}
		info, err := client.StatObject(ctx, bucket, listed.Key, minio.StatObjectOptions{})
		if err != nil {
			return nil, err
		}
		object, err := matchingObject(ctx, client, bucket, listed.Key, info.ETag)
		if err != nil {
			return nil, err
		}
		digest, err := objectDigest(object, info.Size)
		closeErr := object.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		headers := make(map[string]string)
		for _, header := range []string{"Content-Encoding", "Content-Disposition", "Content-Language", "Cache-Control"} {
			headers[header] = info.Metadata.Get(header)
		}
		if !info.Expires.IsZero() {
			headers["Expires"] = info.Expires.UTC().Format(time.RFC3339)
		}
		result = append(result, manifestObject{listed.Key, info.Size, digest, info.ContentType, headers, userMetadata(info)})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	for i := 1; i < len(result); i++ {
		if result[i-1].Key == result[i].Key {
			return nil, errors.New("duplicate key in S3 listing")
		}
	}
	return result, nil
}

func verifyBucket(ctx context.Context, source, target *minio.Client, sourceBucket, targetBucket string, output io.Writer) error {
	expected, err := bucketManifest(ctx, source, sourceBucket)
	if err != nil {
		return err
	}
	actual, err := bucketManifest(ctx, target, targetBucket)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, actual) {
		return errors.New("bucket contents, object counts, bytes or metadata differ")
	}
	encoded, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	return json.NewEncoder(output).Encode(struct {
		SourceBucket   string `json:"source_bucket"`
		TargetBucket   string `json:"target_bucket"`
		Objects        int    `json:"objects"`
		ManifestSHA256 string `json:"manifest_sha256"`
	}{sourceBucket, targetBucket, len(expected), hex.EncodeToString(digest[:])})
}
