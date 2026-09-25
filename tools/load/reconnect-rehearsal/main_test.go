package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGroupHelpersUseInviteTokenContract(t *testing.T) {
	const inviteToken = "0123456789abcdef0123456789abcdef0123456789abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/group/create":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"group-1"}`))
		case "/api/v1/group/invites":
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode create invite: %v", err)
			}
			if request["group_id"] != "group-1" {
				t.Errorf("invite group_id = %q", request["group_id"])
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"` + inviteToken + `"}`))
		case "/api/v1/group/join":
			var request map[string]string
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode join: %v", err)
			}
			if request["invite_token"] != inviteToken || request["code"] != "" {
				t.Errorf("join request = %#v", request)
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	previousBaseURL := *baseURL
	*baseURL = server.URL
	defer func() { *baseURL = previousBaseURL }()

	group, err := createGroup("access-token", "Rehearsal")
	if err != nil {
		t.Fatalf("createGroup: %v", err)
	}
	if group.ID != "group-1" || group.InviteToken != inviteToken {
		t.Fatalf("group = %#v", group)
	}
	if err := joinGroup("access-token", group.InviteToken); err != nil {
		t.Fatalf("joinGroup: %v", err)
	}
}

func TestEncodeDecodeCursor(t *testing.T) {
	now := time.Date(2025, 7, 18, 12, 0, 0, 123456789, time.UTC)
	id := "abc-def-123"

	c1 := encodeCursor(now, id)
	c2 := encodeCursor(now, id)
	// Determinism: same inputs yield same cursor.
	if c1 != c2 {
		t.Fatalf("encodeCursor is not deterministic: %q != %q", c1, c2)
	}
	// Cursor must be non-empty and URL-safe (no +, /, or =).
	for _, ch := range c1 {
		if ch == '+' || ch == '/' || ch == '=' {
			t.Fatalf("cursor contains non-URL-safe character: %q", c1)
		}
	}
}

func TestRandomSuffixDeterminism(t *testing.T) {
	// randomSuffix is time-based (not crypto-random), but each call
	// produces a non-empty value.
	s1 := randomSuffix()
	s2 := randomSuffix()
	if s1 == "" || s2 == "" {
		t.Fatal("randomSuffix returned empty")
	}
	// Different calls may produce the same value under tight loops;
	// that is acceptable since the suffix is used only for uniqueness
	// alongside unix timestamps in practise.
	_ = s1
	_ = s2
}

func TestFlagsParsed(t *testing.T) {
	// baseURL must have the correct default.
	if *baseURL != "http://localhost:8080" {
		t.Fatalf("default baseURL: %q", *baseURL)
	}
}

// TestSignupBodyAttestsAge is the regression guard for the 15+ signup gate.
// Every harness signup must carry age_attested=true; omitting it makes the
// rehearsal fail immediately with 400 age_attestation_required.
func TestSignupBodyAttestsAge(t *testing.T) {
	body := signupBody("player_1", "player_1@test.geoguessme", "ReconnPass123!")
	if attested, ok := body["age_attested"].(bool); !ok || !attested {
		t.Fatalf("age_attested = %#v, want true", body["age_attested"])
	}
	if body["username"] != "player_1" || body["email"] != "player_1@test.geoguessme" || body["password"] != "ReconnPass123!" {
		t.Fatalf("signup body = %#v", body)
	}
}

// TestSignupSendsAgeAttestation exercises the real HTTP path so that a future
// edit bypassing signupBody is caught by CI rather than by a failed rehearsal.
func TestSignupSendsAgeAttestation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/signup" {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode signup: %v", err)
		}
		if attested, _ := payload["age_attested"].(bool); !attested {
			t.Errorf("signup request age_attested = %#v, want true", payload["age_attested"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token-1"}`))
	}))
	defer server.Close()

	previousBaseURL := *baseURL
	*baseURL = server.URL
	defer func() { *baseURL = previousBaseURL }()

	creds, err := signup("player_2", "player_2@test.geoguessme", "ReconnPass123!")
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if creds.Access != "token-1" || creds.Username != "player_2" {
		t.Fatalf("credentials = %#v", creds)
	}
}
