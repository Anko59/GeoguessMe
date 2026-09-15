package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const testToken = "test-access-token"

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	client, err := NewClient(server.URL, testToken, server.Client())
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	return client, server
}

func TestNewClientRejectsUnsafeConfiguration(t *testing.T) {
	for _, test := range []struct {
		name    string
		baseURL string
		token   string
	}{
		{name: "missing token", baseURL: defaultBaseURL},
		{name: "http origin", baseURL: "http://example.test", token: testToken},
		{name: "query", baseURL: "https://example.test?token=bad", token: testToken},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewClient(test.baseURL, test.token, nil); err == nil {
				t.Fatal("NewClient accepted unsafe configuration")
			}
		})
	}
}

func TestGetApplicationSendsBearerAndUsesExpectedEndpoint(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/androidpublisher/v3/applications/com.geoguessme.app" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"packageName":"com.geoguessme.app","title":"GeoGuessMe","defaultLanguageCode":"en-US","appType":"GAME"}`)
	}))
	defer server.Close()

	application, err := client.GetApplication(context.Background(), "com.geoguessme.app")
	if err != nil {
		t.Fatal(err)
	}
	if application.Title != "GeoGuessMe" || application.AppType != "GAME" {
		t.Fatalf("application = %+v", application)
	}
}

func TestInsertEditAndUpdateTrackEncodeRequests(t *testing.T) {
	requests := 0
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/edits") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if len(body) != 0 {
				t.Errorf("insert body = %s", body)
			}
			w.Write([]byte(`{"id":"edit-123","expiryTime":"2026-09-15T20:00:00Z"}`))
			return
		}
		if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/tracks/internal") {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), `"versionCodes":["42"]`) {
				t.Errorf("track body = %s", body)
			}
			w.Write([]byte(`{"track":"internal","releases":[{"versionCodes":["42"],"status":"completed"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	edit, err := client.InsertEdit(context.Background(), "com.geoguessme.app")
	if err != nil || edit.ID != "edit-123" {
		t.Fatalf("insert edit = %+v, err = %v", edit, err)
	}
	track, err := client.UpdateTrack(context.Background(), "com.geoguessme.app", edit.ID, "internal", Track{Track: "internal", Releases: []Release{{VersionCodes: []string{"42"}, Status: "completed"}}})
	if err != nil || len(track.Releases) != 1 {
		t.Fatalf("track = %+v, err = %v", track, err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
}

func TestUploadBundleSetsMediaHeadersAndContentLength(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/upload/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123/bundles" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
		}
		if r.ContentLength != 4 {
			t.Errorf("content length = %d", r.ContentLength)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "AABB" {
			t.Errorf("body = %q", body)
		}
		w.Write([]byte(`{"versionCode":42,"sha256":"hash"}`))
	}))
	defer server.Close()

	bundle, err := client.UploadBundle(context.Background(), "com.geoguessme.app", "edit-123", strings.NewReader("AABB"), 4)
	if err != nil || bundle.VersionCode != 42 {
		t.Fatalf("bundle = %+v, err = %v", bundle, err)
	}
}

func TestValidateAndCommitEditUseDistinctEndpoints(t *testing.T) {
	paths := make([]string, 0, 2)
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.RequestURI())
		w.Write([]byte(`{"id":"edit-123"}`))
	}))
	defer server.Close()

	if _, err := client.ValidateEdit(context.Background(), "com.geoguessme.app", "edit-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CommitEdit(context.Background(), "com.geoguessme.app", "edit-123", true); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123:validate" || paths[1] != "/androidpublisher/v3/applications/com.geoguessme.app/edits/edit-123:commit?changesNotSentForReview=true" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestAPIErrorDoesNotExposeAuthorizationHeader(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"denied"}}`, http.StatusForbidden)
	}))
	defer server.Close()

	_, err := client.GetApplication(context.Background(), "com.geoguessme.app")
	if err == nil || strings.Contains(err.Error(), testToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestJoinURLBuildsExpectedAPIPath(t *testing.T) {
	client, err := NewClient(defaultBaseURL, testToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := client.resourceURL("v3", "applications", "com.example.app")
	if endpoint.EscapedPath() != "/androidpublisher/v3/applications/com.example.app" {
		t.Fatalf("escaped path = %q", endpoint.EscapedPath())
	}
	if _, err := url.Parse(endpoint.String()); err != nil {
		t.Fatal(err)
	}
}
