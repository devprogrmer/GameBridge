// SPDX-License-Identifier: AGPL-3.0-only
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

type Box struct {
	aead cipher.AEAD
}

func New(hexKey string) (*Box, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid hex key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Seal(kind byte, seq uint64, payload []byte) ([]byte, error) {
	plain := make([]byte, 9+len(payload))
	plain[0] = kind
	binary.BigEndian.PutUint64(plain[1:9], seq)
	copy(plain[9:], payload)

	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append([]byte{}, nonce...)
	out = b.aead.Seal(out, nonce, plain, nil)
	return out, nil
}

func (b *Box) Open(packet []byte) (byte, uint64, []byte, error) {
	ns := b.aead.NonceSize()
	if len(packet) < ns+b.aead.Overhead()+9 {
		return 0, 0, nil, errors.New("short encrypted packet")
	}
	nonce := packet[:ns]
	plain, err := b.aead.Open(nil, nonce, packet[ns:], nil)
	if err != nil {
		return 0, 0, nil, err
	}
	if len(plain) < 9 {
		return 0, 0, nil, errors.New("short plaintext")
	}
	return plain[0], binary.BigEndian.Uint64(plain[1:9]), plain[9:], nil
}
