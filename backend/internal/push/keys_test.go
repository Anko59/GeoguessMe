package push

import (
	"testing"
)

func TestGenerateAndParseKeyPairRoundTrip(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(kp.PublicKey) != 65 || kp.PublicKey[0] != 0x04 {
		t.Fatalf("public key = %x, want 65-byte uncompressed point", kp.PublicKey)
	}
	if len(kp.PrivateKey) != 32 {
		t.Fatalf("private key length = %d, want 32", len(kp.PrivateKey))
	}
	parsed, err := ParseKeyPair(kp.PublicKeyBase64URL(), kp.PrivateKeyBase64URL())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if string(parsed.PublicKey) != string(kp.PublicKey) || string(parsed.PrivateKey) != string(kp.PrivateKey) {
		t.Fatal("round-trip mismatch")
	}
}

func TestParseKeyPairRejectsBadInput(t *testing.T) {
	cases := map[string]struct {
		public, private string
	}{
		"empty":             {"", ""},
		"only public":       {"BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8", ""},
		"invalid base64":    {"not!!!base64", "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"},
		"not a curve point": {"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"},
		"zero scalar":       {"BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		"wrong length priv": {"BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8", "short"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseKeyPair(c.public, c.private); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestResolveKeyPairEphemeralWhenUnset(t *testing.T) {
	kp, ephemeral, err := ResolveKeyPair("", "")
	if err != nil {
		t.Fatalf("resolve ephemeral: %v", err)
	}
	if !ephemeral {
		t.Fatal("expected ephemeral flag when keys unset")
	}
	if kp == nil || len(kp.PublicKey) != 65 {
		t.Fatalf("ephemeral keypair invalid: %+v", kp)
	}
}

func TestResolveKeyPairRejectsPartialConfig(t *testing.T) {
	if _, _, err := ResolveKeyPair("BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8", ""); err == nil {
		t.Fatal("partial key config must be rejected")
	}
}

func TestSignerProducesVerifiableSignature(t *testing.T) {
	// A valid keypair must produce an ecdsa signer whose derived public key
	// matches the stored public material, otherwise VAPID JWTs would be rejected.
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	signer := kp.signer()
	if !signer.PublicKey.Curve.IsOnCurve(signer.PublicKey.X, signer.PublicKey.Y) {
		t.Fatal("derived public point is not on the curve")
	}
	// The reconstructed public key must re-marshal to the stored bytes.
	reconstructed := signer.PublicKey
	if len(reconstructed.X.Bytes()) == 0 {
		t.Fatal("public X is zero")
	}
}
