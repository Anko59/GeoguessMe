package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePublishFixtures(t *testing.T, bundleBytes []byte) (string, string) {
	t.Helper()
	directory := t.TempDir()
	bundlePath := filepath.Join(directory, "app-release.aab")
	if err := os.WriteFile(bundlePath, bundleBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(bundleBytes)
	manifest := ReleaseManifest{
		SourceSHA:               strings.Repeat("a", 40),
		SourceTree:              strings.Repeat("b", 40),
		PackageName:             defaultPackageName,
		VersionName:             "0.3.6",
		VersionCode:             3006000,
		AABSHA256:               hex.EncodeToString(digest[:]),
		UploadCertificateSHA256: strings.Repeat("c", 64),
		WorkflowRun:             "35000000000",
	}
	manifestPath := filepath.Join(directory, "android-release-manifest.json")
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return bundlePath, manifestPath
}

func publishOptions(bundlePath, manifestPath string) PublishOptions {
	return PublishOptions{
		PackageName:  defaultPackageName,
		BundlePath:   bundlePath,
		ManifestPath: manifestPath,
		Track:        "internal",
		Status:       "completed",
	}
}

func TestPublishBundleRunsAndVerifiesTheCompleteEditLifecycle(t *testing.T) {
	bundleBytes := []byte("signed release bundle")
	bundlePath, manifestPath := writePublishFixtures(t, bundleBytes)
	requestNumber := 0
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber++
		switch requestNumber {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/androidpublisher/v3/applications/com.geoguessme.app/edits" {
				t.Errorf("insert request = %s %s", r.Method, r.URL.Path)
			}
			io.WriteString(w, `{"id":"edit-123"}`)
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123/tracks" {
				t.Errorf("track list request = %s %s", r.Method, r.URL.Path)
			}
			io.WriteString(w, `{"tracks":[]}`)
		case 3:
			if r.Method != http.MethodPost || r.URL.Path != "/upload/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123/bundles" {
				t.Errorf("upload request = %s %s", r.Method, r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != string(bundleBytes) {
				t.Errorf("uploaded body = %q", body)
			}
			io.WriteString(w, `{"versionCode":3006000}`)
		case 4:
			if r.Method != http.MethodPut || r.URL.Path != "/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123/tracks/internal" {
				t.Errorf("track update request = %s %s", r.Method, r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `"versionCodes":["3006000"]`) || !strings.Contains(string(body), `"status":"completed"`) {
				t.Errorf("track update body = %s", body)
			}
			io.WriteString(w, `{"track":"internal","releases":[{"versionCodes":["3006000"],"status":"completed"}]}`)
		case 5:
			if r.Method != http.MethodPost || r.URL.Path != "/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123:validate" {
				t.Errorf("validate request = %s %s", r.Method, r.URL.Path)
			}
			io.WriteString(w, `{"id":"edit-123"}`)
		case 6:
			if r.Method != http.MethodPost || r.URL.Path != "/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123:commit" || r.URL.RawQuery != "changesInReviewBehavior=ERROR_IF_IN_REVIEW" {
				t.Errorf("commit request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			}
			io.WriteString(w, `{"id":"edit-123"}`)
		case 7:
			if r.Method != http.MethodGet || r.URL.Path != "/androidpublisher/v3/applications/com.geoguessme.app/tracks/internal" {
				t.Errorf("track readback request = %s %s", r.Method, r.URL.Path)
			}
			io.WriteString(w, `{"track":"internal","releases":[{"versionCodes":["3006000"],"status":"completed"}]}`)
		default:
			http.Error(w, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	result, err := publishBundle(context.Background(), client, publishOptions(bundlePath, manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	if requestNumber != 7 {
		t.Fatalf("request count = %d, want 7", requestNumber)
	}
	if result.EditID != "edit-123" || result.VersionCode != 3006000 || result.Track != "internal" {
		t.Fatalf("result = %+v", result)
	}
}

func TestPublishBundleRejectsDigestMismatchBeforeCreatingAnEdit(t *testing.T) {
	bundlePath, manifestPath := writePublishFixtures(t, []byte("signed release bundle"))
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest ReleaseManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.AABSHA256 = strings.Repeat("d", 64)
	manifestData, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}

	requests := 0
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	if _, err := publishBundle(context.Background(), client, publishOptions(bundlePath, manifestPath)); err == nil || !strings.Contains(err.Error(), "does not match manifest") {
		t.Fatalf("err = %v", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}

func TestPublishBundleStopsBeforeTrackUpdateWhenPlayReportsWrongVersion(t *testing.T) {
	bundlePath, manifestPath := writePublishFixtures(t, []byte("signed release bundle"))
	requests := 0
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch requests {
		case 1:
			io.WriteString(w, `{"id":"edit-123"}`)
		case 2:
			io.WriteString(w, `{"tracks":[]}`)
		case 3:
			io.WriteString(w, `{"versionCode":99}`)
		case 4:
			if r.Method != http.MethodDelete || r.URL.Path != "/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123" {
				t.Errorf("cleanup request = %s %s", r.Method, r.URL.Path)
			}
			io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected request %d: %s %s", requests, r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	if _, err := publishBundle(context.Background(), client, publishOptions(bundlePath, manifestPath)); err == nil || !strings.Contains(err.Error(), "expected manifest version code") {
		t.Fatalf("err = %v", err)
	}
	if requests != 4 {
		t.Fatalf("requests = %d, want 4", requests)
	}
}

func TestPublishBundleRejectsUnsupportedStatusBeforeNetwork(t *testing.T) {
	bundlePath, manifestPath := writePublishFixtures(t, []byte("signed release bundle"))
	options := publishOptions(bundlePath, manifestPath)
	options.Status = "published"
	requests := 0
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()

	if _, err := publishBundle(context.Background(), client, options); err == nil || !strings.Contains(err.Error(), "unsupported Play release status") {
		t.Fatalf("err = %v", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}
