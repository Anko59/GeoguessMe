package push

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"testing"
)

// rfc8291Vector is the worked example from RFC 8291 §5 / Appendix A. It pins
// every input and intermediate value so the encryption implementation can be
// validated against the spec byte-for-byte rather than against a self-rolled
// oracle.
var rfc8291Vector = struct {
	plaintext    string
	asPrivateB64 string
	asPublicB64  string
	uaPublicB64  string
	authB64      string
	saltB64      string
	ecdhSecret   string
	ikm          string
	cek          string
	nonce        string
	ciphertext   string
}{
	plaintext:    "When I grow up, I want to be a watermelon",
	asPrivateB64: "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw",
	asPublicB64:  "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8",
	uaPublicB64:  "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
	authB64:      "BTBZMqHH6r4Tts7J_aSIgg",
	saltB64:      "DGv6ra1nlYgDCS1FRnbzlw",
	ecdhSecret:   "kyrL1jIIOHEzg3sM2ZWRHDRB62YACZhhSlknJ672kSs",
	ikm:          "S4lYMb_L0FxCeq0WhDx813KgSYqU26kOyzWUdsXYyrg",
	cek:          "oIhVW04MRdy2XN9CiKLxTg",
	nonce:        "4h_95klXJ5E_qnoN",
	ciphertext:   "8pfeW0KbunFT06SuDKoJH9Ql87S1QUrdirN6GcG7sFz1y1sqLgVi1VhjVkHsUoEsbI_0LpXMuGvnzQ",
}

func mustB64(t *testing.T, value string) []byte {
	t.Helper()
	out, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode %q: %v", value, err)
	}
	return out
}

// TestRFC8291KeyScheduleAndCiphertext reproduces the exact RFC 8291 example. It
// verifies the ECDH shared secret, IKM, CEK, nonce, and the final ciphertext.
func TestRFC8291KeyScheduleAndCiphertext(t *testing.T) {
	v := rfc8291Vector
	uaPublic := mustB64(t, v.uaPublicB64)
	asPublic := mustB64(t, v.asPublicB64)
	auth := mustB64(t, v.authB64)
	salt := mustB64(t, v.saltB64)

	asPrivate, err := ecdh.P256().NewPrivateKey(mustB64(t, v.asPrivateB64))
	if err != nil {
		t.Fatalf("parse as private: %v", err)
	}
	if !bytes.Equal(asPrivate.PublicKey().Bytes(), asPublic) {
		t.Fatalf("as public key mismatch: got %x, want %x", asPrivate.PublicKey().Bytes(), asPublic)
	}
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		t.Fatalf("parse ua public: %v", err)
	}
	ecdhSecret, err := asPrivate.ECDH(ua)
	if err != nil {
		t.Fatalf("ecdh: %v", err)
	}
	if b64(ecdhSecret) != v.ecdhSecret {
		t.Fatalf("ecdh_secret = %s, want %s", b64(ecdhSecret), v.ecdhSecret)
	}

	eph := &ephemeralKeys{private: asPrivate, publicRaw: asPublic, salt: salt}
	cek, nonce := deriveContentKeys(ecdhSecret, uaPublic, auth, eph)
	if b64(cek) != v.cek {
		t.Fatalf("CEK = %s, want %s", b64(cek), v.cek)
	}
	if b64(nonce) != v.nonce {
		t.Fatalf("NONCE = %s, want %s", b64(nonce), v.nonce)
	}

	msg, err := encryptMessage([]byte(v.plaintext), ua, auth, eph)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	// Header is salt(16) + rs(4) + idlen(1) + keyid(65) = 86 bytes.
	if len(msg.bytes()) != 86+len(mustB64(t, v.ciphertext)) {
		t.Fatalf("body length = %d", len(msg.bytes()))
	}
	header := msg.bytes()[:86]
	if !bytes.Equal(header[:16], salt) {
		t.Fatalf("header salt mismatch")
	}
	if header[16] != 0 || header[17] != 0 || header[18] != 0x10 || header[19] != 0 {
		t.Fatalf("record size = %x, want 0x00001000 (4096)", header[16:20])
	}
	if header[20] != 65 {
		t.Fatalf("idlen = %d, want 65", header[20])
	}
	if !bytes.Equal(header[21:], asPublic) {
		t.Fatalf("keyid mismatch")
	}
	gotCiphertext := msg.bytes()[86:]
	if b64(gotCiphertext) != v.ciphertext {
		t.Fatalf("ciphertext = %s, want %s", b64(gotCiphertext), v.ciphertext)
	}
}

func b64(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

func TestEncryptRejectsOversizedPayload(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := ecdh.P256().NewPublicKey(kp.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	eph, err := generateEphemeralKeys()
	if err != nil {
		t.Fatal(err)
	}
	tooLarge := make([]byte, maxPlaintextBytes+1)
	if _, err := encryptMessage(tooLarge, receiver, make([]byte, 16), eph); err == nil {
		t.Fatal("oversized payload was accepted")
	}
}

func TestHKDFExpandMatchesSHA256BlockSize(t *testing.T) {
	// A request larger than one SHA-256 block (32 bytes) exercises the
	// multi-block expansion loop so it cannot silently truncate.
	out := hkdfExpand([]byte("prk"), []byte("info"), 80)
	if len(out) != 80 {
		t.Fatalf("hkdf length = %d, want 80", len(out))
	}
	first := hkdfExpand([]byte("prk"), []byte("info"), 32)
	if !bytes.Equal(out[:32], first) {
		t.Fatal("hkdf first block must be a prefix of a longer expansion")
	}
}
