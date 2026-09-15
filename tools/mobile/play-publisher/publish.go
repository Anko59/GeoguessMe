package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ReleaseManifest is the non-secret contract produced by the verified
// Android release-artifact job. Publication accepts only the exact bundle
// whose digest and source provenance are recorded here.
type ReleaseManifest struct {
	SourceSHA               string `json:"source_sha"`
	SourceTree              string `json:"source_tree"`
	PackageName             string `json:"package_name"`
	VersionName             string `json:"version_name"`
	VersionCode             int64  `json:"version_code"`
	AABSHA256               string `json:"aab_sha256"`
	UploadCertificateSHA256 string `json:"upload_certificate_sha256"`
	WorkflowRun             string `json:"workflow_run"`
}

type PublishOptions struct {
	PackageName  string
	BaseURL      string
	BundlePath   string
	ManifestPath string
	Track        string
	Status       string
}

type PublishResult struct {
	PackageName string `json:"package_name"`
	EditID      string `json:"edit_id"`
	Track       string `json:"track"`
	Status      string `json:"status"`
	VersionName string `json:"version_name"`
	VersionCode int64  `json:"version_code"`
	AABSHA256   string `json:"aab_sha256"`
	SourceSHA   string `json:"source_sha"`
	SourceTree  string `json:"source_tree"`
	WorkflowRun string `json:"workflow_run"`
}

func publishBundle(ctx context.Context, client *Client, options PublishOptions) (PublishResult, error) {
	manifest, err := readReleaseManifest(options.ManifestPath)
	if err != nil {
		return PublishResult{}, err
	}
	if err := validateReleaseManifest(manifest, options.PackageName); err != nil {
		return PublishResult{}, err
	}
	if err := validateTrackStatus(options.Track, options.Status); err != nil {
		return PublishResult{}, err
	}

	bundle, err := os.Open(options.BundlePath)
	if err != nil {
		return PublishResult{}, fmt.Errorf("open Android App Bundle: %w", err)
	}
	defer bundle.Close()
	info, err := bundle.Stat()
	if err != nil {
		return PublishResult{}, fmt.Errorf("stat Android App Bundle: %w", err)
	}
	if !info.Mode().IsRegular() {
		return PublishResult{}, errors.New("Android App Bundle must be a regular file")
	}
	aabSHA256, err := hashBundle(bundle)
	if err != nil {
		return PublishResult{}, fmt.Errorf("hash Android App Bundle: %w", err)
	}
	if aabSHA256 != manifest.AABSHA256 {
		return PublishResult{}, fmt.Errorf("Android App Bundle SHA-256 %s does not match manifest %s", aabSHA256, manifest.AABSHA256)
	}
	if _, err := bundle.Seek(0, io.SeekStart); err != nil {
		return PublishResult{}, fmt.Errorf("rewind Android App Bundle: %w", err)
	}

	application, err := client.GetApplication(ctx, options.PackageName)
	if err != nil {
		return PublishResult{}, err
	}
	if application.PackageName != options.PackageName {
		return PublishResult{}, fmt.Errorf("Play API returned package %q, expected %q", application.PackageName, options.PackageName)
	}
	tracks, err := client.ListTracks(ctx, options.PackageName)
	if err != nil {
		return PublishResult{}, err
	}
	if err := ensureVersionIsNew(tracks, manifest.VersionCode); err != nil {
		return PublishResult{}, err
	}
	edit, err := client.InsertEdit(ctx, options.PackageName)
	if err != nil {
		return PublishResult{}, err
	}
	uploaded, err := client.UploadBundle(ctx, options.PackageName, edit.ID, bundle, info.Size())
	if err != nil {
		return PublishResult{}, err
	}
	if uploaded.VersionCode != manifest.VersionCode {
		return PublishResult{}, fmt.Errorf("Play uploaded version code %d, expected manifest version code %d", uploaded.VersionCode, manifest.VersionCode)
	}

	versionCode := strconv.FormatInt(manifest.VersionCode, 10)
	desired := Track{
		Track: options.Track,
		Releases: []Release{{
			Name:         manifest.VersionName,
			VersionCodes: []string{versionCode},
			Status:       options.Status,
		}},
	}
	if _, err := client.UpdateTrack(ctx, options.PackageName, edit.ID, options.Track, desired); err != nil {
		return PublishResult{}, err
	}
	if _, err := client.ValidateEdit(ctx, options.PackageName, edit.ID); err != nil {
		return PublishResult{}, err
	}
	if _, err := client.CommitEdit(ctx, options.PackageName, edit.ID, false); err != nil {
		return PublishResult{}, err
	}
	current, err := client.GetTrack(ctx, options.PackageName, options.Track)
	if err != nil {
		return PublishResult{}, err
	}
	if !trackContainsRelease(current, versionCode, options.Status) {
		return PublishResult{}, fmt.Errorf("Play track %q did not report version code %s with status %s after commit", options.Track, versionCode, options.Status)
	}

	return PublishResult{
		PackageName: options.PackageName,
		EditID:      edit.ID,
		Track:       options.Track,
		Status:      options.Status,
		VersionName: manifest.VersionName,
		VersionCode: manifest.VersionCode,
		AABSHA256:   manifest.AABSHA256,
		SourceSHA:   manifest.SourceSHA,
		SourceTree:  manifest.SourceTree,
		WorkflowRun: manifest.WorkflowRun,
	}, nil
}

func readReleaseManifest(path string) (ReleaseManifest, error) {
	if strings.TrimSpace(path) == "" {
		return ReleaseManifest{}, errors.New("release manifest path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ReleaseManifest{}, fmt.Errorf("read release manifest: %w", err)
	}
	var manifest ReleaseManifest
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("decode release manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return ReleaseManifest{}, errors.New("release manifest contains trailing JSON")
		}
		return ReleaseManifest{}, fmt.Errorf("decode trailing release manifest data: %w", err)
	}
	return manifest, nil
}

func validateReleaseManifest(manifest ReleaseManifest, packageName string) error {
	if manifest.PackageName != packageName {
		return fmt.Errorf("release manifest package %q does not match %q", manifest.PackageName, packageName)
	}
	if err := validateSHA256(manifest.AABSHA256, "manifest AAB SHA-256"); err != nil {
		return err
	}
	if err := validateSHA256(manifest.UploadCertificateSHA256, "manifest upload certificate SHA-256"); err != nil {
		return err
	}
	if !isGitSHA(manifest.SourceSHA) || !isGitSHA(manifest.SourceTree) {
		return errors.New("release manifest source SHA and tree must be 40-character hexadecimal Git IDs")
	}
	if strings.TrimSpace(manifest.VersionName) == "" {
		return errors.New("release manifest version name is required")
	}
	if manifest.VersionCode <= 0 {
		return errors.New("release manifest version code must be positive")
	}
	if strings.TrimSpace(manifest.WorkflowRun) == "" {
		return errors.New("release manifest workflow run is required")
	}
	return nil
}

func validateTrackStatus(track, status string) error {
	if err := validatePathPart(track, "track"); err != nil {
		return err
	}
	switch status {
	case "draft", "inProgress", "halted", "completed":
		return nil
	default:
		return fmt.Errorf("unsupported Play release status %q", status)
	}
}

func validateSHA256(value, name string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("%s must be a 64-character hexadecimal value", name)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("%s must be hexadecimal: %w", name, err)
	}
	if value != strings.ToLower(value) {
		return fmt.Errorf("%s must be lowercase", name)
	}
	return nil
}

func isGitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func hashBundle(bundle *os.File) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, bundle); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func trackContainsRelease(track Track, versionCode, status string) bool {
	for _, release := range track.Releases {
		if release.Status != status {
			continue
		}
		for _, code := range release.VersionCodes {
			if code == versionCode {
				return true
			}
		}
	}
	return false
}

func ensureVersionIsNew(tracks []Track, versionCode int64) error {
	for _, track := range tracks {
		for _, release := range track.Releases {
			for _, code := range release.VersionCodes {
				existing, err := strconv.ParseInt(code, 10, 64)
				if err != nil || existing <= 0 {
					return fmt.Errorf("Play returned invalid version code %q in track %q", code, track.Track)
				}
				if existing >= versionCode {
					return fmt.Errorf("release version code %d is not newer than Play track %q version code %d", versionCode, track.Track, existing)
				}
			}
		}
	}
	return nil
}
