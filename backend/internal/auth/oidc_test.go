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
