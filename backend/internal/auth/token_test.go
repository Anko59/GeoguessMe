package auth

import (
	"testing"
	"time"
)

func TestAccessTokenClaimsAreShortLivedAndTyped(t *testing.T) {
	InitWithSettings("a-strong-test-secret-that-is-at-least-32-bytes", "issuer", "audience", 15*time.Minute)
	token, err := GenerateAccessToken("user-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ValidateAccessToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "user-1" || claims.TokenType != "access" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.ExpiresAt == nil || time.Until(claims.ExpiresAt.Time) > 15*time.Minute || time.Until(claims.ExpiresAt.Time) < 14*time.Minute {
		t.Fatalf("unexpected expiry: %v", claims.ExpiresAt)
	}
}

func TestOpaqueTokensAreHashableAndUnpredictable(t *testing.T) {
	first, err := GenerateOpaqueToken(48)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateOpaqueToken(48)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || HashToken(first) == first || HashToken(first) == HashToken(second) {
		t.Fatal("opaque token generation/hash failed")
	}
}
