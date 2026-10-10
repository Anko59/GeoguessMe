package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/minio/minio-go/v7"
)

// Migration copies current object bytes and HTTP/user metadata only. It does not
// copy bucket policies, ACLs, historical versions, timestamps or object tags.
// Stop all source and destination writers before invoking this explicit command.
func copyBucket(ctx context.Context, source, target *minio.Client, sourceBucket, targetBucket string, output io.Writer) error {
	exists, err := source.BucketExists(ctx, sourceBucket)
	if err != nil {
		return err
	}
	if !exists {
		return errors.New("source bucket does not exist")
	}
	if err := ensureBucket(ctx, target, targetBucket); err != nil {
		return err
	}
	for listed := range source.ListObjects(ctx, sourceBucket, minio.ListObjectsOptions{Recursive: true}) {
		if listed.Err != nil {
			return listed.Err
		}
		status, digest, err := copyObject(ctx, source, target, sourceBucket, targetBucket, listed.Key)
		if err != nil {
			return fmt.Errorf("copy object %q: %w", listed.Key, err)
		}
		if err := json.NewEncoder(output).Encode(struct {
			Key    string `json:"key"`
			Status string `json:"status"`
			SHA256 string `json:"sha256"`
		}{listed.Key, status, digest}); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func copyObject(ctx context.Context, source, target *minio.Client, sourceBucket, targetBucket, key string) (string, string, error) {
	sourceInfo, err := source.StatObject(ctx, sourceBucket, key, minio.StatObjectOptions{})
	if err != nil {
		return "", "", err
	}
	if sourceInfo.Size > maxSinglePutSize || sourceInfo.Size < 0 {
		return "", "", errors.New("object exceeds 5 GiB conditional single-PUT limit")
	}
	sourceObject, err := matchingObject(ctx, source, sourceBucket, key, sourceInfo.ETag)
	if err != nil {
		return "", "", err
	}
	defer sourceObject.Close()
	expected, err := objectDigest(sourceObject, sourceInfo.Size)
	if err != nil {
		return "", "", err
	}
	targetInfo, err := target.StatObject(ctx, targetBucket, key, minio.StatObjectOptions{})
	if err == nil {
		if err := verifyObject(ctx, target, targetBucket, key, targetInfo, sourceInfo, expected); err != nil {
			return "", "", fmt.Errorf("existing destination conflicts; refusing overwrite: %w", err)
		}
		return "unchanged", expected, nil
	}
	if code := minio.ToErrorResponse(err).Code; code != "NoSuchKey" && code != "NoSuchObject" && code != "NotFound" {
		return "", "", err
	}
	if _, err := sourceObject.Seek(0, io.SeekStart); err != nil {
		return "", "", err
	}
	options := minio.PutObjectOptions{
		UserMetadata: sourceInfo.UserMetadata, ContentType: sourceInfo.ContentType,
		ContentEncoding:    sourceInfo.Metadata.Get("Content-Encoding"),
		ContentDisposition: sourceInfo.Metadata.Get("Content-Disposition"),
		ContentLanguage:    sourceInfo.Metadata.Get("Content-Language"),
		CacheControl:       sourceInfo.Metadata.Get("Cache-Control"), Expires: sourceInfo.Expires,
		DisableMultipart: true, DisableContentSha256: true, SendContentMd5: true,
	}
	// Conditional atomic PUT avoids overwriting a concurrently created key on
	// compatible S3 servers; an operator must still quiesce both stores first.
	options.SetMatchETagExcept("*")
	if _, err := target.PutObject(ctx, targetBucket, key, sourceObject, sourceInfo.Size, options); err != nil {
		return "", "", err
	}
	targetInfo, err = target.StatObject(ctx, targetBucket, key, minio.StatObjectOptions{})
	if err != nil {
		return "", "", err
	}
	if err := verifyObject(ctx, target, targetBucket, key, targetInfo, sourceInfo, expected); err != nil {
		return "", "", err
	}
	return "copied", expected, nil
}

func matchingObject(ctx context.Context, client *minio.Client, bucket, key, etag string) (*minio.Object, error) {
	options := minio.GetObjectOptions{}
	if err := options.SetMatchETag(etag); err != nil {
		return nil, err
	}
	return client.GetObject(ctx, bucket, key, options)
}

func objectDigest(reader io.Reader, size int64) (string, error) {
	hash := sha256.New()
	read, err := io.Copy(hash, reader)
	if err != nil {
		return "", err
	}
	if read != size {
		return "", fmt.Errorf("object length changed: expected %d, read %d", size, read)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func userMetadata(info minio.ObjectInfo) map[string]string {
	result := make(map[string]string)
	for key, value := range info.UserMetadata {
		result[strings.ToLower(strings.TrimPrefix(strings.ToLower(key), "x-amz-meta-"))] = value
	}
	return result
}

func sameMetadata(left, right minio.ObjectInfo) bool {
	if left.Size != right.Size || left.ContentType != right.ContentType ||
		!left.Expires.Equal(right.Expires) || !reflect.DeepEqual(userMetadata(left), userMetadata(right)) {
		return false
	}
	for _, header := range []string{"Content-Encoding", "Content-Disposition", "Content-Language", "Cache-Control"} {
		if left.Metadata.Get(header) != right.Metadata.Get(header) {
			return false
		}
	}
	return true
}

func verifyObject(ctx context.Context, client *minio.Client, bucket, key string, actual, expected minio.ObjectInfo, digest string) error {
	if !sameMetadata(actual, expected) {
		return errors.New("object size or content/user metadata differs")
	}
	object, err := matchingObject(ctx, client, bucket, key, actual.ETag)
	if err != nil {
		return err
	}
	defer object.Close()
	actualDigest, err := objectDigest(object, actual.Size)
	if err != nil {
		return err
	}
	if actualDigest != digest {
		return errors.New("destination SHA-256 differs from source")
	}
	return nil
}
