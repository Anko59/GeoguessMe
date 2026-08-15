package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestOIDCVerifierRequiresSignedAudienceBoundVerifiedEmail(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := ""
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeOIDCTestJSON(t, w, map[string]string{"issuer": issuer, "jwks_uri": issuer + "/keys"})
		case "/keys":
			writeOIDCTestJSON(t, w, map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": "test-key", "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(privateKey.E)).Bytes()),
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	issuer = provider.URL

	verifier, err := NewOIDCVerifier(context.Background(), issuer, "geoguessme-web")
	if err != nil {
		t.Fatal(err)
	}
	valid := signedIdentityToken(t, privateKey, issuer, "geoguessme-web", true)
	identity, err := verifier.VerifyIdentity(context.Background(), "Bearer "+valid)
	if err != nil || identity.Subject != "keycloak-user-1" || identity.Email != "alice@example.test" || identity.PreferredUsername != "Alice.Example" {
		t.Fatalf("VerifyIdentity(valid) = %+v, %v", identity, err)
	}

	for name, authorization := range map[string]string{
		"missing bearer": valid,
		"wrong audience": "Bearer " + signedIdentityToken(t, privateKey, issuer, "other-client", true),
		"unverified":     "Bearer " + signedIdentityToken(t, privateKey, issuer, "geoguessme-web", false),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifier.VerifyIdentity(context.Background(), authorization); err == nil {
				t.Fatal("invalid identity was accepted")
			}
		})
	}
}

func TestOIDCVerifierDeletesOnlyMatchingKeycloakIdentity(t *testing.T) {
	var issuer string
	var tokenRequested, identityDeleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/geoguessme/protocol/openid-connect/token":
			tokenRequested = true
			if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "geoguessme-dev" || r.Form.Get("client_secret") != "client-secret" {
				t.Errorf("unexpected service token request: %v %v", err, r.Form)
			}
			writeOIDCTestJSON(t, w, map[string]string{"access_token": "service-token"})
		case "/admin/realms/geoguessme/users/keycloak-user-1":
			identityDeleted = true
			if r.Method != http.MethodDelete || r.Header.Get("Authorization") != "Bearer service-token" {
				t.Errorf("unexpected identity deletion request: %s %q", r.Method, r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL + "/realms/geoguessme"
	verifier := &OIDCVerifier{
		issuer: issuer, clientID: "geoguessme-dev", clientSecret: "client-secret", httpClient: server.Client(),
	}
	if err := verifier.DeleteIdentity(context.Background(), issuer, "keycloak-user-1"); err != nil {
		t.Fatalf("DeleteIdentity: %v", err)
	}
	if !tokenRequested || !identityDeleted {
		t.Fatalf("token requested = %v, identity deleted = %v", tokenRequested, identityDeleted)
	}

	tokenRequested = false
	if err := verifier.DeleteIdentity(context.Background(), issuer+"-other", "keycloak-user-1"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("issuer mismatch = %v, want configuration error", err)
	}
	if tokenRequested {
		t.Fatal("issuer mismatch reached Keycloak")
	}
}

func TestKeycloakAdminUserURLPreservesContextPath(t *testing.T) {
	got, err := keycloakAdminUserURL("https://login.example.test/auth/realms/geoguessme", "subject-1")
	if err != nil || got != "https://login.example.test/auth/admin/realms/geoguessme/users/subject-1" {
		t.Fatalf("keycloakAdminUserURL = %q, %v", got, err)
	}
}

func signedIdentityToken(t *testing.T, key *rsa.PrivateKey, issuer, audience string, emailVerified bool) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "aud": audience, "sub": "keycloak-user-1",
		"email": "alice@example.test", "email_verified": emailVerified,
		"preferred_username": "Alice.Example",
		"iat":                time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func writeOIDCTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode identity fixture: %v", err)
	}
}
