package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/curve25519"
)

// [A state-of-the-art Diffie-Hellman function](https://cr.yp.to/ecdh.html)

func init() {
	if curve25519.PointSize != 32 {
		panic(`invalid point size`)
	}
	if curve25519.ScalarSize != 32 {
		panic(`invalid scalar size`)
	}
}

var base64codec = base64.URLEncoding.WithPadding(base64.NoPadding)

type PublicKey [32]byte

func PublicKeyFromString(s string) (PublicKey, error) {
	b, err := base64codec.DecodeString(s)
	if err != nil {
		return PublicKey{}, err
	}
	if len(b) != 32 {
		return PublicKey{}, fmt.Errorf(`invalid point size`)
	}
	k := PublicKey{}
	copy(k[:], b)
	return k, nil
}

func (k PublicKey) String() string {
	return base64codec.EncodeToString(k[:])
}

type PrivateKey [32]byte

func PrivateKeyFromString(s string) (PrivateKey, error) {
	b, err := base64codec.DecodeString(s)
	if err != nil {
		return PrivateKey{}, err
	}
	if len(b) != 32 {
		return PrivateKey{}, fmt.Errorf(`invalid point size`)
	}
	k := PrivateKey{}
	copy(k[:], b)
	return k, nil
}

func (k PrivateKey) String() string {
	return base64codec.EncodeToString(k[:])
}

func NewPrivateKey() PrivateKey {
	k := PrivateKey{}
	n, err := rand.Read(k[:])
	if n != 32 || err != nil {
		panic(fmt.Sprintf(`rand.read error: n=%d,err=%v`, n, err))
	}
	return k
}

func (k PrivateKey) PublicKey() PublicKey {
	pub, err := curve25519.X25519(k[:], curve25519.Basepoint)
	if err != nil {
		panic(`invalid basepoint`)
	}

	pub2 := PublicKey{}
	copy(pub2[:], pub)
	return pub2
}

type SharedSecret [32]byte

func (k PrivateKey) SharedSecret(other PublicKey) SharedSecret {
	sharedBytes, err := curve25519.X25519(k[:], other[:])
	if err != nil {
		panic(`invalid point for other`)
	}
	shared := SharedSecret{}
	copy(shared[:], sharedBytes)
	return shared
}

type AesGcm struct {
	aead cipher.AEAD
}

func NewAesGcm(sharedSecret SharedSecret) (*AesGcm, error) {
	block, err := aes.NewCipher(sharedSecret[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AesGcm{aead: gcm}, nil
}

// data will be overwritten.
func (b *AesGcm) Encrypt(data []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := b.aead.Seal(data[:0], nonce, data, nil)
	return append(sealed, nonce...), nil
}

// data will be overwritten.
func (b *AesGcm) Decrypt(data []byte) ([]byte, error) {
	if len(data) < b.aead.NonceSize() {
		return nil, fmt.Errorf(`invalid data to decrypt`)
	}
	nonce := make([]byte, b.aead.NonceSize())
	pos := len(data) - b.aead.NonceSize()
	if n := copy(nonce, data[pos:]); n != b.aead.NonceSize() {
		panic(`internal error: nonce size mismatch`)
	}
	return b.aead.Open(data[:0], nonce, data[:pos], nil)
}
