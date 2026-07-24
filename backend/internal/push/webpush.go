package push

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// recordSize is the default "rs" value for aes128gcm (RFC 8188). A single push
// record therefore carries up to rs-1(delimiter)-16(tag) plaintext bytes.
const recordSize uint32 = 4096

// maxPlaintextBytes is the largest payload a single-record push message can
// protect. Push services are not required to accept more, so larger payloads
// are rejected before encryption rather than producing an undeliverable message.
const maxPlaintextBytes = int(recordSize) - 1 - 16

// info labels per RFC 8291 §3.4. Each is NUL-terminated as the HKDF info field.
var (
	cekInfo   = []byte("Content-Encoding: aes128gcm\x00")
	nonceInfo = []byte("Content-Encoding: nonce\x00")
)

// encryptedMessage is a fully serialized aes128gcm push body ready to POST to a
// push service endpoint as the request body.
type encryptedMessage struct {
	body []byte
}

// bytes returns the serialized header || ciphertext.
func (m encryptedMessage) bytes() []byte { return m.body }

// ephemeralKeys holds the per-message application-server ECDH keypair and the
// random header salt. Production callers generate a fresh pair per message;
// tests inject the RFC 8291 test vector values.
type ephemeralKeys struct {
	private   *ecdh.PrivateKey
	publicRaw []byte // 65-byte uncompressed point (also the aes128gcm keyid)
	salt      []byte // 16 bytes
}

// generateEphemeralKeys mints a fresh ECDH keypair and salt for one message.
func generateEphemeralKeys() (*ephemeralKeys, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral ECDH key: %w", err)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate push salt: %w", err)
	}
	return &ephemeralKeys{private: priv, publicRaw: priv.PublicKey().Bytes(), salt: salt}, nil
}

// encryptMessage protects payload end-to-end for a single subscription using
// RFC 8291 (WebPush) over RFC 8188 (aes128gcm). The receiver's p256dh public
// key and auth secret are the subscription credentials; eph provides the
// per-message keypair and salt.
func encryptMessage(payload []byte, receiver *ecdh.PublicKey, authSecret []byte, eph *ephemeralKeys) (*encryptedMessage, error) {
	if len(payload) > maxPlaintextBytes {
		return nil, fmt.Errorf("payload of %d bytes exceeds the %d-byte push limit", len(payload), maxPlaintextBytes)
	}
	if len(authSecret) != 16 {
		return nil, fmt.Errorf("auth secret must be 16 bytes, got %d", len(authSecret))
	}

	ecdhSecret, err := eph.private.ECDH(receiver)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}

	cek, nonce := deriveContentKeys(ecdhSecret, receiver.Bytes(), authSecret, eph)

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// Single record, final delimiter 0x02 per RFC 8291 §4.
	record := make([]byte, 0, len(payload)+1)
	record = append(record, payload...)
	record = append(record, 0x02)
	ciphertext := gcm.Seal(nil, nonce, record, nil)

	header := make([]byte, 0, 21+len(eph.publicRaw))
	header = append(header, eph.salt...)
	var rs [4]byte
	binary.BigEndian.PutUint32(rs[:], recordSize)
	header = append(header, rs[:]...)
	header = append(header, byte(len(eph.publicRaw)))
	header = append(header, eph.publicRaw...)

	body := make([]byte, 0, len(header)+len(ciphertext))
	body = append(body, header...)
	body = append(body, ciphertext...)
	return &encryptedMessage{body: body}, nil
}

// deriveContentKeys performs the RFC 8291 key schedule: it mixes the ECDH
// shared secret with the authentication secret and header salt to produce the
// AES-128-GCM content encryption key and nonce. The intermediate IKM is also
// returned so the RFC 8291 test vector can assert every stage.
func deriveContentKeys(ecdhSecret, receiverPublicKey, authSecret []byte, eph *ephemeralKeys) (cek, nonce []byte) {
	// RFC 8291 §3.3: combine the ECDH and authentication secrets into the IKM.
	prkKey := hmacSHA256(authSecret, ecdhSecret)
	keyInfo := bytes.Join([][]byte{[]byte("WebPush: info\x00"), receiverPublicKey, eph.publicRaw}, nil)
	ikm := hkdfExpand(prkKey, keyInfo, 32)
	// RFC 8188 content key derivation using the header salt.
	prk := hmacSHA256(eph.salt, ikm)
	return hkdfExpand(prk, cekInfo, 16), hkdfExpand(prk, nonceInfo, 12)
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// hkdfExpand is the RFC 5869 Expand step (SHA-256). It is implemented directly
// rather than via crypto/hkdf so the exact NUL-terminated info fields used by
// RFC 8291 are unambiguous and dependency-free.
func hkdfExpand(prk, info []byte, length int) []byte {
	out := make([]byte, 0, length)
	var previous []byte
	for counter := byte(1); len(out) < length; counter++ {
		mac := hmac.New(sha256.New, prk)
		mac.Write(previous)
		mac.Write(info)
		mac.Write([]byte{counter})
		previous = mac.Sum(nil)
		out = append(out, previous...)
	}
	return out[:length]
}
