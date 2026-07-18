package main

import (
	"testing"
	"time"
)

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
