package auth

import (
	"testing"
	"time"
)

func TestAccessTokenClaimsAreShortLivedAndTyped(t *testing.T) {
	service := NewService("a-strong-test-secret-that-is-at-least-32-bytes", "issuer", "audience", 15*time.Minute)
	token, err := service.GenerateAccessToken("user-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.ValidateAccessToken(token)
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

func TestServiceInstancesAreIndependent(t *testing.T) {
	serviceA := NewService("secret-A-that-is-at-least-32-bytes-long", "issuer-a", "audience-a", 15*time.Minute)
	serviceB := NewService("secret-B-that-is-at-least-32-bytes-long", "issuer-b", "audience-b", 15*time.Minute)
	tokenA, err := serviceA.GenerateAccessToken("user-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	tokenB, err := serviceB.GenerateAccessToken("user-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	// Each service only validates its own tokens: cross-service validation
	// fails because the signing keys differ.
	if _, err := serviceB.ValidateAccessToken(tokenA); err == nil {
		t.Fatal("service B accepted a token signed by service A")
	}
	if _, err := serviceA.ValidateAccessToken(tokenB); err == nil {
		t.Fatal("service A accepted a token signed by service B")
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
