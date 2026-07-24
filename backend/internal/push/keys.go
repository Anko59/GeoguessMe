// Package push implements end-to-end Web Push (VAPID + RFC 8188 aes128gcm)
// for GeoGuessMe without any third-party push provider or native wrapper. It
// owns VAPID key handling, message encryption, subscription storage, delivery,
// and the notification fan-out service.
package push

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
)

// KeyPair is an application VAPID P-256 keypair. PublicKey is the 65-byte
// uncompressed point (0x04 || X || Y) that browsers receive and use to scope
// subscriptions. PrivateKey is the 32-byte big-endian scalar D used to sign
// VAPID JWTs. Both are the canonical encodings push services expect.
type KeyPair struct {
	PublicKey  []byte // 65 bytes
	PrivateKey []byte // 32 bytes
}

// PublicKeyBase64URL returns the unpadded base64url encoding of the public key,
// the form Application Server keys are exchanged in.
func (k *KeyPair) PublicKeyBase64URL() string {
	return base64.RawURLEncoding.EncodeToString(k.PublicKey)
}

// PrivateKeyBase64URL returns the unpadded base64url encoding of the private
// scalar, suitable for storage in an environment variable.
func (k *KeyPair) PrivateKeyBase64URL() string {
	return base64.RawURLEncoding.EncodeToString(k.PrivateKey)
}

// signer reconstructs the ecdsa.PrivateKey used to sign VAPID JWTs (ES256).
// The public point is derived deterministically from the scalar so the stored
// private material is sufficient.
func (k *KeyPair) signer() *ecdsa.PrivateKey {
	d := new(big.Int).SetBytes(k.PrivateKey)
	curve := elliptic.P256()
	priv := &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: curve}, D: d}
	priv.PublicKey.X, priv.PublicKey.Y = curve.ScalarBaseMult(k.PrivateKey)
	return priv
}

// GenerateKeyPair creates a fresh VAPID keypair using the cryptographic random
// source. It backs the `vapid-keys` CLI subcommand and ephemeral dev keys.
func GenerateKeyPair() (*KeyPair, error) {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate VAPID keypair: %w", err)
	}
	public := key.PublicKey().Bytes()
	if len(public) != 65 {
		return nil, fmt.Errorf("unexpected public key length %d", len(public))
	}
	return &KeyPair{PublicKey: public, PrivateKey: key.Bytes()}, nil
}

// ParseKeyPair decodes the base64url VAPID keys an operator stores in the
// environment. The public key must be the 65-byte uncompressed point and the
// private key the 32-byte scalar; both length and curve validity are checked.
func ParseKeyPair(publicB64, privateB64 string) (*KeyPair, error) {
	if publicB64 == "" || privateB64 == "" {
		return nil, fmt.Errorf("VAPID public and private keys are required")
	}
	pub, err := base64.RawURLEncoding.DecodeString(publicB64)
	if err != nil {
		return nil, fmt.Errorf("VAPID public key must be unpadded base64url: %w", err)
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil || len(pub) != 65 || pub[0] != 0x04 {
		return nil, fmt.Errorf("VAPID public key is not a valid P-256 uncompressed point")
	}
	priv, err := base64.RawURLEncoding.DecodeString(privateB64)
	if err != nil {
		return nil, fmt.Errorf("VAPID private key must be unpadded base64url: %w", err)
	}
	d := new(big.Int).SetBytes(priv)
	if d.Sign() == 0 || d.Cmp(elliptic.P256().Params().N) >= 0 {
		return nil, fmt.Errorf("VAPID private key scalar is out of range")
	}
	if len(priv) != 32 {
		return nil, fmt.Errorf("VAPID private key must be 32 bytes, got %d", len(priv))
	}
	return &KeyPair{PublicKey: pub, PrivateKey: priv}, nil
}

// ResolveKeyPair returns the configured keypair, or a fresh ephemeral one when
// neither value is set. Mixed (one set, one empty) configurations are an error.
// Ephemeral keys let development and test stacks run without operator setup;
// production validation requires explicit keys so subscriptions survive restarts.
func ResolveKeyPair(publicB64, privateB64 string) (*KeyPair, bool, error) {
	if publicB64 == "" && privateB64 == "" {
		kp, err := GenerateKeyPair()
		return kp, true, err
	}
	kp, err := ParseKeyPair(publicB64, privateB64)
	return kp, false, err
}
