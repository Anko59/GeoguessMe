package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// baseURL is the gateway the isolated test stack publishes. It can be overridden
// with TEST_BASE_URL so the suite can target a developer-managed stack too.
var baseURL string

func TestMain(m *testing.M) {
	baseURL = os.Getenv("TEST_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if !waitForReady(baseURL, 60*time.Second) {
		fmt.Fprintf(os.Stderr, "integration suite requires a ready server at %s (set TEST_BASE_URL)\n", baseURL)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func waitForReady(base string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/health/ready", nil)
		if err == nil {
			resp, err := client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true
				}
			}
		}
		time.Sleep(time.Second)
	}
	return false
}

// --- HTTP helpers ---------------------------------------------------------

type tokenPair struct {
	access  string
	refresh *http.Cookie
	userID  string
}

type jsonResponse struct {
	StatusCode int
	Header     http.Header
	cookies    []*http.Cookie
}

func (r jsonResponse) Cookies() []*http.Cookie {
	return r.cookies
}

func doJSON(t *testing.T, method, path string, body any, bearer string, cookies []*http.Cookie) (jsonResponse, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, baseURL+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return jsonResponse{StatusCode: resp.StatusCode, Header: resp.Header, cookies: resp.Cookies()}, data
}

func signup(t *testing.T, username, email, password string) tokenPair {
	t.Helper()
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": username, "email": email, "password": password}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "signup status %d: %s", resp.StatusCode, data)
	var result struct {
		AccessToken string `json:"access_token"`
		User        struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(data, &result))
	require.NotEmpty(t, result.AccessToken)
	var refresh *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "refresh_token" {
			refresh = c
		}
	}
	return tokenPair{access: result.AccessToken, refresh: refresh, userID: result.User.ID}
}

func createGroup(t *testing.T, bearer string, name string) (id, code string) {
	t.Helper()
	resp, data := doJSON(t, http.MethodPost, "/api/v1/group/create", map[string]string{"name": name}, bearer, nil)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "create group %d: %s", resp.StatusCode, data)
	var result struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(data, &result))
	return result.ID, result.Code
}

func joinGroup(t *testing.T, bearer, code string) {
	t.Helper()
	resp, data := doJSON(t, http.MethodPost, "/api/v1/group/join", map[string]string{"code": code}, bearer, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "join group %d: %s", resp.StatusCode, data)
}

// uploadPhoto uploads a 1x1 PNG and returns the new challenge id.
func uploadPhoto(t *testing.T, bearer, groupID string) string {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "test.png")
	require.NoError(t, err)
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
	require.NoError(t, err)
	_, err = part.Write(image)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("lat", "51.505"))
	require.NoError(t, writer.WriteField("long", "-0.09"))
	require.NoError(t, writer.WriteField("group_id", groupID))
	require.NoError(t, writer.Close())

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/photo/upload", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "upload %d: %s", resp.StatusCode, data)
	var result struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &result))
	return result.ID
}

// serverNow reads the gateway's HTTP Date header so tests can wait on server
// time without relying on local clock drift.
func serverNow(t *testing.T) time.Time {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/health/ready", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	tm, err := http.ParseTime(resp.Header.Get("Date"))
	require.NoError(t, err)
	return tm
}

// acceptChallenge records the server-controlled viewing window returned by the
// accept endpoint without mutating challenge state.
type acceptance struct {
	PhotoID       string
	MediaURL      string
	ViewExpiresAt time.Time
	ServerTime    time.Time
}

func acceptChallenge(t *testing.T, bearer, photoID string) acceptance {
	t.Helper()
	resp, data := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/accept", nil, bearer, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "accept %d: %s", resp.StatusCode, data)
	var body struct {
		PhotoID       string `json:"photo_id"`
		MediaURL      string `json:"media_url"`
		ViewExpiresAt string `json:"view_expires_at"`
		ServerTime    string `json:"server_time"`
	}
	require.NoError(t, jsonUnmarshal(data, &body))
	viewExp, err := time.Parse(time.RFC3339Nano, body.ViewExpiresAt)
	require.NoError(t, err)
	serverT, err := time.Parse(time.RFC3339Nano, body.ServerTime)
	require.NoError(t, err)
	return acceptance{PhotoID: body.PhotoID, MediaURL: body.MediaURL, ViewExpiresAt: viewExp, ServerTime: serverT}
}

// waitUntilViewExpires blocks until the server clock passes the stored view
// deadline (plus a small grace period) WITHOUT submitting a guess.
func waitUntilViewExpires(t *testing.T, deadline time.Time) {
	t.Helper()
	grace := 300 * time.Millisecond
	limit := time.Now().Add(30 * time.Second)
	for time.Now().Before(limit) {
		if serverNow(t).After(deadline.Add(-grace)) {
			time.Sleep(grace)
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatal("viewing window did not close before the test deadline")
}

func guess(t *testing.T, bearer, photoID string, lat, long float64) int {
	t.Helper()
	resp, data := doJSON(t, http.MethodPost, "/api/v1/challenges/"+photoID+"/guess",
		map[string]float64{"lat": lat, "long": long}, bearer, nil)
	require.Containsf(t, []int{http.StatusCreated, http.StatusOK}, resp.StatusCode, "guess %d: %s", resp.StatusCode, data)
	return resp.StatusCode
}

// --- Mailpit helper -------------------------------------------------------

func mailpitBase() string {
	if v := os.Getenv("MAILPIT_BASE_URL"); v != "" {
		return v
	}
	// Default test stack publishes Mailpit API on :8025.
	u, err := url.Parse(baseURL)
	if err == nil {
		return fmt.Sprintf("http://%s:8025", u.Hostname())
	}
	return "http://localhost:8025"
}

// tokenFromMailpit reads the most recent message to an address and extracts the
// last path token from the link embedded in the body.
func tokenFromMailpit(t *testing.T, email, linkPath string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	deadline := time.Now().Add(10 * time.Second)
	searchURL := mailpitBase() + "/api/v1/search"
	queryURL, _ := url.Parse(searchURL)
	queryVals := queryURL.Query()
	queryVals.Set("query", "to:"+email)
	queryURL.RawQuery = queryVals.Encode()
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, queryURL.String(), nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			var summary struct {
				Messages []struct {
					ID string `json:"ID"`
				} `json:"messages"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&summary)
			_ = resp.Body.Close()
			if len(summary.Messages) > 0 {
				id := summary.Messages[0].ID
				req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, mailpitBase()+"/api/v1/message/"+id, nil)
				r2, err := http.DefaultClient.Do(req2)
				if err == nil {
					var msg struct {
						Text string `json:"Text"`
					}
					_ = json.NewDecoder(r2.Body).Decode(&msg)
					_ = r2.Body.Close()
					if tok := extractToken(msg.Text, linkPath); tok != "" {
						return tok
					}
				}
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("no mailpit message for %s containing %s", email, linkPath)
	return ""
}

func extractToken(body, linkPath string) string {
	idx := indexOf(body, linkPath+"?token=")
	if idx < 0 {
		return ""
	}
	start := idx + len(linkPath) + len("?token=")
	end := start
	for end < len(body) && isTokenChar(body[end]) {
		end++
	}
	return body[start:end]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func isTokenChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

var uniqueMu sync.Mutex
var uniqueCounter int64

// unique returns a per-run unique handle to keep credentials from colliding
// when the test suite runs repeatedly against a persistent stack.
func unique(prefix string) string {
	uniqueMu.Lock()
	defer uniqueMu.Unlock()
	uniqueCounter++
	return fmt.Sprintf("%s%d%d", prefix, time.Now().UnixNano(), uniqueCounter)
}

// --- Toxiproxy helpers ---------------------------------------------------

func toxiproxyBase() string {
	if v := os.Getenv("TOXIPROXY_API_URL"); v != "" {
		return v
	}
	return "http://localhost:8474"
}

// addToxiproxyTimeout adds a timeout toxic to the named proxy. A timeout of
// zero causes immediate connection drops, simulating a storage outage.
func addToxiproxyTimeout(t *testing.T, proxyName string, timeoutMs int) {
	t.Helper()
	body := fmt.Sprintf(`{"name":"storage_outage","type":"timeout","stream":"downstream","toxicity":1,"attributes":{"timeout":%d}}`, timeoutMs)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		toxiproxyBase()+"/proxies/"+proxyName+"/toxics", bytes.NewReader([]byte(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "toxiproxy timeout toxic must be added")
}

// removeToxiproxyTimeout removes the storage_outage timeout toxic from the
// named proxy, restoring normal operation.
func removeToxiproxyTimeout(t *testing.T, proxyName string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete,
		toxiproxyBase()+"/proxies/"+proxyName+"/toxics/storage_outage", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	// 204 No Content on success; 404 if already removed (idempotent).
	require.True(t, resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound,
		"toxiproxy timeout toxic removal must succeed")
}

// waitForReadyStatus polls /health/ready until it returns the expected status
// or the deadline is reached.
func waitForReadyStatus(t *testing.T, expectedStatus int) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	client := &http.Client{Timeout: 15 * time.Second}
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/health/ready", nil)
		require.NoError(t, err)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == expectedStatus {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("/health/ready did not reach status %d within deadline", expectedStatus)
}

// countPhotos returns the number of photo rows across all lifecycle states.
func countPhotos(t *testing.T) int {
	t.Helper()
	db := testDB(t)
	var n int
	require.NoError(t, db.QueryRow(t.Context(), `SELECT COUNT(*) FROM photos`).Scan(&n))
	return n
}
