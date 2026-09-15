package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://androidpublisher.googleapis.com"

// Client is the small REST transport used by the release workflow. It accepts
// an already-issued OAuth access token so credential issuance remains the
// responsibility of the caller (OIDC in CI or a local credential helper).
type Client struct {
	baseURL     *url.URL
	accessToken string
	httpClient  *http.Client
}

// NewClient creates an Android Publisher API client. The token is kept only in
// memory and is sent in the Authorization header, never in a URL or log line.
func NewClient(baseURL, accessToken string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("Play API access token is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 2 * time.Minute}
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Play API base URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("Play API base URL must be an HTTPS origin without query or fragment")
	}
	return &Client{baseURL: parsed, accessToken: accessToken, httpClient: httpClient}, nil
}

type Application struct {
	PackageName         string `json:"packageName"`
	Title               string `json:"title"`
	DefaultLanguageCode string `json:"defaultLanguageCode"`
	AppType             string `json:"appType"`
}

type AppEdit struct {
	ID         string `json:"id"`
	ExpiryTime string `json:"expiryTime"`
	Kind       string `json:"kind"`
}

type Bundle struct {
	VersionCode int64  `json:"versionCode"`
	SHA1        string `json:"sha1"`
	SHA256      string `json:"sha256"`
}

type LocalizedText struct {
	Language string `json:"language"`
	Text     string `json:"text"`
}

type Release struct {
	Name         string          `json:"name,omitempty"`
	VersionCodes []string        `json:"versionCodes"`
	ReleaseNotes []LocalizedText `json:"releaseNotes,omitempty"`
	Status       string          `json:"status"`
	UserFraction float64         `json:"userFraction,omitempty"`
}

type Track struct {
	Track    string    `json:"track"`
	Releases []Release `json:"releases"`
}

type TrackList struct {
	Tracks []Track `json:"tracks"`
}

// GetApplication performs the read-only identity check used before any edit.
func (c *Client) GetApplication(ctx context.Context, packageName string) (Application, error) {
	if err := validatePathPart(packageName, "package name"); err != nil {
		return Application{}, err
	}
	var application Application
	err := c.doJSON(ctx, http.MethodGet, c.resourceURL("v3", "applications", packageName), nil, &application)
	if err != nil {
		return Application{}, fmt.Errorf("get application %q: %w", packageName, err)
	}
	return application, nil
}

// ListTracks reads every committed track so publication can reject a bundle
// whose version code was already uploaded anywhere in the app.
func (c *Client) ListTracks(ctx context.Context, packageName string) ([]Track, error) {
	if err := validatePathPart(packageName, "package name"); err != nil {
		return nil, err
	}
	var tracks TrackList
	err := c.doJSON(ctx, http.MethodGet, c.resourceURL("v3", "applications", packageName, "tracks"), nil, &tracks)
	if err != nil {
		return nil, fmt.Errorf("list Play tracks: %w", err)
	}
	return tracks.Tracks, nil
}

// InsertEdit creates a new Play edit transaction.
func (c *Client) InsertEdit(ctx context.Context, packageName string) (AppEdit, error) {
	if err := validatePathPart(packageName, "package name"); err != nil {
		return AppEdit{}, err
	}
	var edit AppEdit
	err := c.doJSON(ctx, http.MethodPost, c.resourceURL("v3", "applications", packageName, "edits"), nil, &edit)
	if err != nil {
		return AppEdit{}, fmt.Errorf("create Play edit: %w", err)
	}
	if strings.TrimSpace(edit.ID) == "" {
		return AppEdit{}, errors.New("create Play edit returned no edit ID")
	}
	return edit, nil
}

// UploadBundle uploads one signed Android App Bundle into an existing edit.
func (c *Client) UploadBundle(ctx context.Context, packageName, editID string, bundle io.Reader, size int64) (Bundle, error) {
	if err := validatePathPart(packageName, "package name"); err != nil {
		return Bundle{}, err
	}
	if err := validatePathPart(editID, "edit ID"); err != nil {
		return Bundle{}, err
	}
	if bundle == nil {
		return Bundle{}, errors.New("bundle reader is required")
	}
	if size < 0 {
		return Bundle{}, errors.New("bundle size must not be negative")
	}
	endpoint := c.uploadURL("v3", "applications", packageName, "edits", editID, "bundles")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bundle)
	if err != nil {
		return Bundle{}, fmt.Errorf("create bundle upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = size
	var uploaded Bundle
	if err := c.sendJSON(req, &uploaded); err != nil {
		return Bundle{}, fmt.Errorf("upload Android App Bundle: %w", err)
	}
	if uploaded.VersionCode <= 0 {
		return Bundle{}, errors.New("bundle upload returned no version code")
	}
	return uploaded, nil
}

// UpdateTrack replaces the desired state of a track in an existing edit.
func (c *Client) UpdateTrack(ctx context.Context, packageName, editID, track string, desired Track) (Track, error) {
	for value, name := range map[string]string{packageName: "package name", editID: "edit ID", track: "track"} {
		if err := validatePathPart(value, name); err != nil {
			return Track{}, err
		}
	}
	var updated Track
	err := c.doJSON(ctx, http.MethodPut, c.resourceURL("v3", "applications", packageName, "edits", editID, "tracks", track), desired, &updated)
	if err != nil {
		return Track{}, fmt.Errorf("update Play track %q: %w", track, err)
	}
	return updated, nil
}

// GetTrack reads the committed state of a track. Callers use this after an
// edit commit to verify that Play accepted the intended version and status.
func (c *Client) GetTrack(ctx context.Context, packageName, track string) (Track, error) {
	for value, name := range map[string]string{packageName: "package name", track: "track"} {
		if err := validatePathPart(value, name); err != nil {
			return Track{}, err
		}
	}
	var current Track
	err := c.doJSON(ctx, http.MethodGet, c.resourceURL("v3", "applications", packageName, "tracks", track), nil, &current)
	if err != nil {
		return Track{}, fmt.Errorf("get Play track %q: %w", track, err)
	}
	return current, nil
}

// ValidateEdit asks Play to validate all changes in an edit without publishing.
func (c *Client) ValidateEdit(ctx context.Context, packageName, editID string) (AppEdit, error) {
	for value, name := range map[string]string{packageName: "package name", editID: "edit ID"} {
		if err := validatePathPart(value, name); err != nil {
			return AppEdit{}, err
		}
	}
	var edit AppEdit
	err := c.doJSON(ctx, http.MethodPost, c.resourceURL("v3", "applications", packageName, "edits", editID+":validate"), nil, &edit)
	if err != nil {
		return AppEdit{}, fmt.Errorf("validate Play edit: %w", err)
	}
	return edit, nil
}

// CommitEdit publishes a validated edit. It refuses to cancel another edit
// already in review; the release workflow must handle that state explicitly.
func (c *Client) CommitEdit(ctx context.Context, packageName, editID string, changesNotSentForReview bool) (AppEdit, error) {
	for value, name := range map[string]string{packageName: "package name", editID: "edit ID"} {
		if err := validatePathPart(value, name); err != nil {
			return AppEdit{}, err
		}
	}
	var edit AppEdit
	endpoint := c.resourceURL("v3", "applications", packageName, "edits", editID+":commit")
	query := endpoint.Query()
	query.Set("changesInReviewBehavior", "ERROR_IF_IN_REVIEW")
	if changesNotSentForReview {
		query.Set("changesNotSentForReview", "true")
	}
	endpoint.RawQuery = query.Encode()
	err := c.doJSON(ctx, http.MethodPost, endpoint, nil, &edit)
	if err != nil {
		return AppEdit{}, fmt.Errorf("commit Play edit: %w", err)
	}
	return edit, nil
}

func (c *Client) resourceURL(parts ...string) *url.URL {
	return c.joinURL(append([]string{"androidpublisher"}, parts...)...)
}

func (c *Client) uploadURL(parts ...string) *url.URL {
	return c.joinURL(append([]string{"upload", "androidpublisher"}, parts...)...)
}

func (c *Client) joinURL(parts ...string) *url.URL {
	joined := c.baseURL.JoinPath(parts...)
	if !strings.HasPrefix(joined.Path, "/") {
		joined.Path = "/" + joined.Path
	}
	return joined
}

func validatePathPart(value, name string) error {
	if value == "" {
		return fmt.Errorf("Play API %s is required", name)
	}
	if strings.ContainsAny(value, "/?#") {
		return fmt.Errorf("Play API %s contains an unsafe path character", name)
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method string, endpoint *url.URL, body any, result any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.sendJSON(req, result)
}

func (c *Client) sendJSON(req *http.Request, result any) error {
	response, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return apiError(response)
	}
	if result == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func apiError(response *http.Response) error {
	const maxErrorBody = 4096
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxErrorBody+1))
	if len(body) > maxErrorBody {
		body = append(body[:maxErrorBody], []byte("...")...)
	}
	message := strings.TrimSpace(string(body))
	if readErr != nil {
		message = fmt.Sprintf("unable to read error body: %v", readErr)
	}
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return fmt.Errorf("Play API returned HTTP %d: %s", response.StatusCode, message)
}
