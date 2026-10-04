package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixtureObject struct {
	body     []byte
	metadata http.Header
}

type fixtureServer struct {
	mu       sync.Mutex
	buckets  map[string]map[string]fixtureObject
	requests []string
	deny     bool
	corrupt  bool
	pageSize int
}

func s3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, "<Error><Code>%s</Code><Message>fixture error</Message></Error>", code)
}

func (s *fixtureServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r.Method+" "+r.URL.Path)
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") ||
		!strings.Contains(r.Header.Get("Authorization"), "/us-east-1/s3/aws4_request") || s.deny {
		s3Error(w, http.StatusForbidden, "AccessDenied")
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)
	bucket := parts[0]
	key := ""
	if len(parts) > 1 {
		key = parts[1]
	}
	objects, exists := s.buckets[bucket]
	if key == "" {
		if r.Method == http.MethodPut {
			if !exists {
				s.buckets[bucket] = make(map[string]fixtureObject)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if !exists {
			s3Error(w, http.StatusNotFound, "NoSuchBucket")
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodGet {
			type item struct {
				Key          string
				Size         int
				ETag         string
				LastModified string
			}
			result := struct {
				XMLName               xml.Name `xml:"ListBucketResult"`
				Name                  string
				IsTruncated           bool
				NextContinuationToken string
				Contents              []item
			}{Name: bucket}
			keys := make([]string, 0, len(objects))
			for key := range objects {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			start := 0
			if token := r.URL.Query().Get("continuation-token"); token != "" {
				position := sort.SearchStrings(keys, token)
				if position >= len(keys) || keys[position] != token {
					s3Error(w, http.StatusBadRequest, "InvalidToken")
					return
				}
				start = position + 1
			}
			end := len(keys)
			if s.pageSize > 0 && start+s.pageSize < end {
				end = start + s.pageSize
				result.IsTruncated = true
				result.NextContinuationToken = keys[end-1]
			}
			for _, key := range keys[start:end] {
				object := objects[key]
				sum := md5.Sum(object.body)
				result.Contents = append(result.Contents, item{key, len(object.body), hex.EncodeToString(sum[:]), "2026-01-01T00:00:00Z"})
			}
			w.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(w).Encode(result)
			return
		}
	}
	if !exists {
		s3Error(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	object, found := objects[key]
	if r.Method == http.MethodPut {
		if r.Header.Get("If-None-Match") != "*" {
			s3Error(w, http.StatusBadRequest, "InvalidRequest")
			return
		}
		if found {
			s3Error(w, http.StatusPreconditionFailed, "PreconditionFailed")
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			s3Error(w, http.StatusBadRequest, "InvalidRequest")
			return
		}
		if s.corrupt && len(body) > 0 {
			body[0] ^= 1
		}
		metadata := make(http.Header)
		for name, values := range r.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-amz-meta-") ||
				name == "Content-Type" || name == "Content-Encoding" || name == "Content-Disposition" ||
				name == "Content-Language" || name == "Cache-Control" || name == "Expires" {
				metadata[name] = values
			}
		}
		objects[key] = fixtureObject{body, metadata}
		sum := md5.Sum(body)
		w.Header().Set("ETag", "\""+hex.EncodeToString(sum[:])+"\"")
		w.WriteHeader(http.StatusOK)
		return
	}
	if !found {
		s3Error(w, http.StatusNotFound, "NoSuchKey")
		return
	}
	sum := md5.Sum(object.body)
	etag := "\"" + hex.EncodeToString(sum[:]) + "\""
	if match := r.Header.Get("If-Match"); match != "" && match != etag {
		s3Error(w, http.StatusPreconditionFailed, "PreconditionFailed")
		return
	}
	for key, values := range object.metadata {
		w.Header()[key] = values
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Last-Modified", "Thu, 01 Jan 2026 00:00:00 GMT")
	w.Header().Set("Content-Length", strconv.Itoa(len(object.body)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method == http.MethodGet {
		_, _ = w.Write(object.body)
		return
	}
	s3Error(w, http.StatusBadRequest, "InvalidRequest")
}

func newFixtureServer(t *testing.T) (*fixtureServer, string) {
	t.Helper()
	state := &fixtureServer{buckets: make(map[string]map[string]fixtureObject)}
	server := httptest.NewServer(http.HandlerFunc(state.serve))
	t.Cleanup(server.Close)
	return state, server.URL
}

func fixtureEnv(endpoint string) map[string]string {
	return map[string]string{"S3_FIXTURE_ENDPOINT": endpoint, "S3_FIXTURE_ACCESS_KEY": "fixture", "S3_FIXTURE_SECRET_KEY": "fixture-secret"}
}

func runFixture(t *testing.T, env map[string]string, input string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := execute(ctx, args, func(key string) string { return env[key] }, strings.NewReader(input), &output)
	return output.String(), err
}

func TestFixtureOperations(t *testing.T) {
	_, endpoint := newFixtureServer(t)
	env := fixtureEnv(endpoint)
	for range 2 {
		if _, err := runFixture(t, env, "", "ensure", "fixture-bucket"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runFixture(t, env, "rehearsal payload", "put", "fixture-bucket", "folder/payload", "-"); err != nil {
		t.Fatal(err)
	}
	if _, err := runFixture(t, env, "replacement", "put", "fixture-bucket", "folder/payload", "-"); err == nil {
		t.Fatal("overwrite was accepted")
	}
	body, err := runFixture(t, env, "", "get", "fixture-bucket", "folder/payload")
	if err != nil || body != "rehearsal payload" {
		t.Fatalf("get=%q, error=%v", body, err)
	}
	head, err := runFixture(t, env, "", "head", "fixture-bucket", "folder/payload")
	if err != nil || !json.Valid([]byte(head)) || !strings.Contains(head, "\"size\":17") {
		t.Fatalf("head=%s, error=%v", head, err)
	}
	list, err := runFixture(t, env, "", "list", "fixture-bucket")
	if err != nil || !strings.Contains(list, "folder/payload") {
		t.Fatalf("list=%s, error=%v", list, err)
	}
	if _, err := runFixture(t, env, "", "get", "fixture-bucket", "missing"); err == nil {
		t.Fatal("missing object was accepted")
	}
}

func TestFixtureValidation(t *testing.T) {
	for _, endpoint := range []string{"", "http://user:secret@localhost", "http://localhost/path", "http://localhost?token=secret", "file:///tmp/object", "http://localhost/#fragment"} {
		if _, err := newClient(func(key string) string { return fixtureEnv(endpoint)[key] }, ""); err == nil {
			t.Fatalf("accepted endpoint %q", endpoint)
		}
	}
	client, err := newClient(func(key string) string { return fixtureEnv("http://localhost")[key] }, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := matchingObject(context.Background(), client, "bucket", "key", ""); err == nil {
		t.Fatal("missing source ETag silently disabled source-change protection")
	}
	if _, err := newClient(func(key string) string {
		if key == "S3_FIXTURE_ENDPOINT" {
			return "http://localhost"
		}
		return ""
	}, ""); err == nil {
		t.Fatal("missing credentials accepted")
	}
	if err := execute(context.Background(), []string{"unknown", "bucket"}, func(string) string { return "" }, nil, io.Discard); err == nil {
		t.Fatal("unknown operation accepted")
	}
	if err := execute(context.Background(), []string{"put", "bucket", "key"}, func(string) string { return "" }, nil, io.Discard); err == nil {
		t.Fatal("missing file accepted")
	}
	state, endpoint := newFixtureServer(t)
	state.deny = true
	if _, err := runFixture(t, fixtureEnv(endpoint), "", "ensure", "fixture-bucket"); err == nil {
		t.Fatal("authorization failure ignored")
	}
}

func TestFixtureCopy(t *testing.T) {
	source, sourceURL := newFixtureServer(t)
	target, targetURL := newFixtureServer(t)
	metadata := http.Header{"Content-Type": {"image/jpeg"}, "Cache-Control": {"private, max-age=120"}, "X-Amz-Meta-Owner": {"alice"}}
	source.buckets["source-bucket"] = map[string]fixtureObject{
		"photos/a.jpg": {[]byte("photo payload"), metadata},
		"empty":        {[]byte{}, http.Header{"Content-Type": {"application/octet-stream"}}},
	}
	source.pageSize, target.pageSize = 1, 1
	env := fixtureEnv(targetURL)
	for key, value := range fixtureEnv(sourceURL) {
		env["SOURCE_"+key] = value
	}
	for _, expected := range []string{"copied", "unchanged"} {
		output, err := runFixture(t, env, "", "copy", "source-bucket", "target-bucket")
		if err != nil || !strings.Contains(output, "\"status\":\""+expected+"\"") {
			t.Fatalf("copy=%s, error=%v", output, err)
		}
	}
	verification, err := runFixture(t, env, "", "verify", "source-bucket", "target-bucket")
	if err != nil || !strings.Contains(verification, "\"objects\":2") || !strings.Contains(verification, "manifest_sha256") {
		t.Fatalf("verify=%s, error=%v", verification, err)
	}
	source.mu.Lock()
	requests := append([]string(nil), source.requests...)
	sourceData := source.buckets["source-bucket"]["photos/a.jpg"]
	source.mu.Unlock()
	for _, request := range requests {
		if !strings.HasPrefix(request, "GET ") && !strings.HasPrefix(request, "HEAD ") {
			t.Fatalf("source mutated: %s", request)
		}
	}
	target.mu.Lock()
	object := target.buckets["target-bucket"]["photos/a.jpg"]
	object.body = []byte("other payload")
	target.buckets["target-bucket"]["photos/a.jpg"] = object
	target.mu.Unlock()
	if _, err := runFixture(t, env, "", "copy", "source-bucket", "target-bucket"); err == nil {
		t.Fatal("different destination bytes were overwritten")
	}
	target.mu.Lock()
	preserved := string(target.buckets["target-bucket"]["photos/a.jpg"].body)
	target.mu.Unlock()
	if preserved != "other payload" {
		t.Fatal("conflicting destination mutated")
	}
	if _, err := runFixture(t, env, "", "verify", "source-bucket", "target-bucket"); err == nil {
		t.Fatal("conflicting destination verified")
	}
	// Even matching source keys cannot hide an unexpected destination object.
	target.mu.Lock()
	target.buckets["target-bucket"]["photos/a.jpg"] = sourceData
	target.buckets["target-bucket"]["extra"] = fixtureObject{[]byte("extra"), http.Header{"Content-Type": {"text/plain"}}}
	target.mu.Unlock()
	if _, err := runFixture(t, env, "", "verify", "source-bucket", "target-bucket"); err == nil {
		t.Fatal("extra destination key ignored")
	}
}

func TestFixtureCopyDetectsCorruptionAndMetadataConflicts(t *testing.T) {
	source, sourceURL := newFixtureServer(t)
	target, targetURL := newFixtureServer(t)
	source.buckets["source-bucket"] = map[string]fixtureObject{"key": {[]byte("data"), http.Header{"Content-Type": {"text/plain"}}}}
	env := fixtureEnv(targetURL)
	for key, value := range fixtureEnv(sourceURL) {
		env["SOURCE_"+key] = value
	}
	target.corrupt = true
	if _, err := runFixture(t, env, "", "copy", "source-bucket", "target-bucket"); err == nil {
		t.Fatal("corrupt transfer accepted")
	}
	target.mu.Lock()
	target.corrupt = false
	target.buckets["target-bucket"]["key"] = fixtureObject{[]byte("data"), http.Header{"Content-Type": {"application/octet-stream"}}}
	target.mu.Unlock()
	if _, err := runFixture(t, env, "", "copy", "source-bucket", "target-bucket"); err == nil {
		t.Fatal("different destination metadata accepted")
	}
	env["SOURCE_S3_FIXTURE_ENDPOINT"] = targetURL
	if _, err := runFixture(t, env, "", "copy", "target-bucket", "target-bucket"); err == nil {
		t.Fatal("same source and destination accepted")
	}
}

func TestParseFixtureArguments(t *testing.T) {
	tests := []struct {
		name    string
		cli     []string
		payload string
		want    []string
		invalid bool
	}{
		{name: "legacy zero"},
		{name: "legacy CLI", cli: []string{"get", "bucket", "a b/$()"}, want: []string{"get", "bucket", "a b/$()"}},
		{name: "typed spaces and metacharacters", payload: `["put","bucket","photos/space \"quote\" $() $HOME 漢.jpg","/tmp/file with spaces \"$()\".jpg"]`, want: []string{"put", "bucket", "photos/space \"quote\" $() $HOME 漢.jpg", "/tmp/file with spaces \"$()\".jpg"}},
		{name: "JSON whitespace", payload: " \n [ \"list\", \"bucket\" ] \t", want: []string{"list", "bucket"}},
		{name: "malformed", payload: `["get",`, invalid: true},
		{name: "trailing JSON", payload: `["list","bucket"] ["get"]`, invalid: true},
		{name: "trailing garbage", payload: `["list","bucket"] extra`, invalid: true},
		{name: "root object", payload: `{}`, invalid: true},
		{name: "root null", payload: `null`, invalid: true},
		{name: "empty array", payload: `[]`, invalid: true},
		{name: "too many strings", payload: `["put","bucket","key","file","extra"]`, invalid: true},
		{name: "number", payload: `["get","bucket",42]`, invalid: true},
		{name: "boolean", payload: `["get","bucket",true]`, invalid: true},
		{name: "null entry", payload: `["get","bucket",null]`, invalid: true},
		{name: "object entry", payload: `["get","bucket",{}]`, invalid: true},
		{name: "array entry", payload: `["get","bucket",[]]`, invalid: true},
		{name: "ambiguous", cli: []string{"list", "bucket"}, payload: `["list","bucket"]`, invalid: true},
		{name: "oversized", payload: `["` + strings.Repeat("x", 16*1024) + `"]`, invalid: true},
		{name: "exact size boundary", payload: `["` + strings.Repeat("x", 16*1024-4) + `"]`, want: []string{strings.Repeat("x", 16*1024-4)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed, err := parseArguments(test.cli, func(string) string { return test.payload })
			if test.invalid {
				if err == nil {
					t.Fatal("invalid argument source accepted")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(parsed, test.want) {
				t.Fatalf("parsed=%q, error=%v", parsed, err)
			}
		})
	}
}

func TestJSONArgumentsPreserveObjectKeysAndFilePaths(t *testing.T) {
	_, endpoint := newFixtureServer(t)
	env := fixtureEnv(endpoint)
	if _, err := runFixture(t, env, "", "ensure", "fixture-bucket"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), `photo with spaces "$()".jpg`)
	if err := os.WriteFile(path, []byte("literal path payload"), 0600); err != nil {
		t.Fatal(err)
	}
	key := `photos/space "quote" $() $HOME.jpg`
	payload, err := json.Marshal([]string{"put", "fixture-bucket", key, path})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseArguments(nil, func(string) string { return string(payload) })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runFixture(t, env, "", parsed...); err != nil {
		t.Fatal(err)
	}
	body, err := runFixture(t, env, "", "get", "fixture-bucket", key)
	if err != nil || body != "literal path payload" {
		t.Fatalf("get=%q, error=%v", body, err)
	}
}
