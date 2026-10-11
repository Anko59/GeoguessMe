// S3 fixture operations reuse the application's maintained S3 client library.
// This is local test infrastructure, not an application or hosted R2 client.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const maxSinglePutSize int64 = 5 * 1024 * 1024 * 1024

func newClient(getenv func(string) string, prefix string) (*minio.Client, error) {
	endpoint, err := url.Parse(getenv(prefix + "S3_FIXTURE_ENDPOINT"))
	if err != nil || endpoint == nil || endpoint.Host == "" ||
		(endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil ||
		(endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" {
		return nil, errors.New("S3 fixture endpoint must be an explicit HTTP(S) origin without credentials, path or query")
	}
	access := getenv(prefix + "S3_FIXTURE_ACCESS_KEY")
	secret := getenv(prefix + "S3_FIXTURE_SECRET_KEY")
	if access == "" || secret == "" {
		return nil, errors.New("S3 fixture access and secret keys are required")
	}
	region := getenv(prefix + "S3_FIXTURE_REGION")
	if region == "" {
		region = "us-east-1"
	}
	return minio.New(endpoint.Host, &minio.Options{
		Creds: credentials.NewStaticV4(access, secret, ""), Secure: endpoint.Scheme == "https",
		Region: region, BucketLookup: minio.BucketLookupPath,
	})
}

func ensureBucket(ctx context.Context, client *minio.Client, bucket string) error {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil || exists {
		return err
	}
	if err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		code := minio.ToErrorResponse(err).Code
		if code != "BucketAlreadyOwnedByYou" && code != "BucketAlreadyExists" {
			return err
		}
		exists, checkErr := client.BucketExists(ctx, bucket)
		if checkErr != nil {
			return checkErr
		}
		if !exists {
			return err
		}
	}
	return nil
}

func putFixture(ctx context.Context, client *minio.Client, bucket, key, path string, input io.Reader) error {
	var reader io.ReadSeeker
	var size int64
	if path == "-" {
		// Rehearsal seed input is small. Bound it rather than starting an unknown-
		// size multipart upload or accepting unlimited stdin into memory.
		data, err := io.ReadAll(io.LimitReader(input, 16*1024*1024+1))
		if err != nil {
			return err
		}
		if len(data) > 16*1024*1024 {
			return errors.New("fixture stdin exceeds 16 MiB; use a file")
		}
		reader, size = bytes.NewReader(data), int64(len(data))
	} else {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("fixture input must be a regular file")
		}
		reader, size = file, info.Size()
	}
	if size > maxSinglePutSize {
		return errors.New("fixture object exceeds 5 GiB conditional single-PUT limit")
	}
	options := minio.PutObjectOptions{ContentType: "application/octet-stream", DisableMultipart: true,
		DisableContentSha256: true, SendContentMd5: true}
	options.SetMatchETagExcept("*")
	_, err := client.PutObject(ctx, bucket, key, reader, size, options)
	return err
}

func execute(ctx context.Context, args []string, getenv func(string) string, input io.Reader, output io.Writer) error {
	if len(args) < 2 {
		return errors.New("expected ensure BUCKET, put BUCKET KEY FILE|-, head BUCKET KEY, get BUCKET KEY, list BUCKET, copy SOURCE_BUCKET TARGET_BUCKET or verify SOURCE_BUCKET TARGET_BUCKET")
	}
	operation, bucket := args[0], args[1]
	expected := map[string]int{"ensure": 2, "put": 4, "head": 3, "get": 3, "list": 2, "copy": 3, "verify": 3}
	if count, ok := expected[operation]; !ok || count != len(args) {
		return errors.New("unknown S3 fixture operation or incorrect argument count")
	}
	client, err := newClient(getenv, "")
	if err != nil {
		return err
	}
	switch operation {
	case "ensure":
		return ensureBucket(ctx, client, bucket)
	case "put":
		return putFixture(ctx, client, bucket, args[2], args[3], input)
	case "head":
		info, err := client.StatObject(ctx, bucket, args[2], minio.StatObjectOptions{})
		if err != nil {
			return err
		}
		return json.NewEncoder(output).Encode(info)
	case "get":
		object, err := client.GetObject(ctx, bucket, args[2], minio.GetObjectOptions{})
		if err != nil {
			return err
		}
		defer object.Close()
		_, err = io.Copy(output, object)
		return err
	case "list":
		for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if object.Err != nil {
				return object.Err
			}
			if err := json.NewEncoder(output).Encode(object); err != nil {
				return err
			}
		}
		return ctx.Err()
	case "copy", "verify":
		source, err := newClient(getenv, "SOURCE_")
		if err != nil {
			return err
		}
		if source.EndpointURL().String() == client.EndpointURL().String() && bucket == args[2] {
			return errors.New("source and destination must be different")
		}
		if operation == "verify" {
			return verifyBucket(ctx, source, client, bucket, args[2], output)
		}
		return copyBucket(ctx, source, client, bucket, args[2], output)
	}
	return errors.New("unreachable S3 fixture operation")
}

// JSON arguments are data, never a shell command. The optional typed interface
// preserves whitespace and metacharacters without changing legacy CLI parsing.
func parseArguments(args []string, getenv func(string) string) ([]string, error) {
	payload := getenv("S3_FIXTURE_ARGS_JSON")
	if payload == "" {
		return args, nil
	}
	if len(args) != 0 {
		return nil, errors.New("choose either S3_FIXTURE_ARGS_JSON or CLI arguments, not both")
	}
	if len(payload) > 16*1024 {
		return nil, errors.New("S3_FIXTURE_ARGS_JSON exceeds 16 KiB")
	}
	var values []any
	if err := json.Unmarshal([]byte(payload), &values); err != nil {
		return nil, errors.New("S3_FIXTURE_ARGS_JSON must be one JSON array of strings")
	}
	if len(values) == 0 || len(values) > 4 {
		return nil, errors.New("S3_FIXTURE_ARGS_JSON must contain one to four strings")
	}
	parsed := make([]string, len(values))
	for index, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, errors.New("S3_FIXTURE_ARGS_JSON entries must be strings")
		}
		parsed[index] = text
	}
	return parsed, nil
}

func main() {
	args, err := parseArguments(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "S3 fixture arguments rejected:", err)
		os.Exit(1)
	}
	timeout := 2 * time.Minute
	if len(args) > 0 && (strings.EqualFold(args[0], "copy") || strings.EqualFold(args[0], "verify")) {
		timeout = 2 * time.Hour
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := execute(ctx, args, os.Getenv, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "S3 fixture operation failed:", err)
		os.Exit(1)
	}
}
