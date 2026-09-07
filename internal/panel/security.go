// SPDX-License-Identifier: AGPL-3.0-only
package panel

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const passwordIterations = 210000

func randomHex(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hLen := 32
	blocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, blocks*hLen)
	for block := 1; block <= blocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		var ctr [4]byte
		binary.BigEndian.PutUint32(ctr[:], uint32(block))
		mac.Write(ctr[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
func hashPassword(p string) (string, error) {
	if len(p) < 10 {
		return "", errors.New("password must be at least 10 characters")
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	dk := pbkdf2SHA256([]byte(p), salt, passwordIterations, 32)
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", passwordIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(dk)), nil
}
func verifyPassword(enc, p string) bool {
	parts := strings.Split(enc, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	it, e := strconv.Atoi(parts[1])
	if e != nil || it < 100000 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[2])
	if e != nil {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(parts[3])
	if e != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(p), salt, it, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}
func ensureKey(path string) ([]byte, error) {
	if b, e := os.ReadFile(path); e == nil {
		raw, e := hex.DecodeString(strings.TrimSpace(string(b)))
		if e == nil && len(raw) == 32 {
			return raw, nil
		}
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return nil, e
	}
	raw := make([]byte, 32)
	if _, e := rand.Read(raw); e != nil {
		return nil, e
	}
	if e := os.WriteFile(path, []byte(hex.EncodeToString(raw)+"\n"), 0600); e != nil {
		return nil, e
	}
	return raw, nil
}

type Crypt struct{ key []byte }

func NewCrypt(k []byte) *Crypt { return &Crypt{key: k} }
func (c *Crypt) Seal(plain string) (string, error) {
	b, e := aes.NewCipher(c.key)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return "", e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", e
	}
	ct := g.Seal(nil, n, []byte(plain), nil)
	return base64.RawURLEncoding.EncodeToString(append(n, ct...)), nil
}
func (c *Crypt) Open(enc string) (string, error) {
	all, e := base64.RawURLEncoding.DecodeString(enc)
	if e != nil {
		return "", e
	}
	b, e := aes.NewCipher(c.key)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(b)
	if e != nil {
		return "", e
	}
	if len(all) < g.NonceSize() {
		return "", errors.New("invalid encrypted value")
	}
	pt, e := g.Open(nil, all[:g.NonceSize()], all[g.NonceSize():], nil)
	return string(pt), e
}

type sessionPayload struct {
	AdminID  string `json:"admin_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
}

func signSession(k []byte, p sessionPayload) (string, error) {
	raw, e := json.Marshal(p)
	if e != nil {
		return "", e
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	m := hmac.New(sha256.New, k)
	m.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}
func verifySession(k []byte, t string) (sessionPayload, bool) {
	var p sessionPayload
	parts := strings.Split(t, ".")
	if len(parts) != 2 {
		return p, false
	}
	m := hmac.New(sha256.New, k)
	m.Write([]byte(parts[0]))
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil || !hmac.Equal(m.Sum(nil), sig) {
		return p, false
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil || json.Unmarshal(raw, &p) != nil || p.Exp < time.Now().Unix() {
		return sessionPayload{}, false
	}
	return p, true
}
func newTOTPSecret() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
}
func verifyTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if secret == "" {
		return true
	}
	if len(code) != 6 {
		return false
	}
	raw, e := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if e != nil {
		return false
	}
	step := now.Unix() / 30
	for d := int64(-1); d <= 1; d++ {
		var msg [8]byte
		binary.BigEndian.PutUint64(msg[:], uint64(step+d))
		m := hmac.New(sha1.New, raw)
		m.Write(msg[:])
		sum := m.Sum(nil)
		o := sum[len(sum)-1] & 0x0f
		bin := (uint32(sum[o])&0x7f)<<24 | (uint32(sum[o+1])&0xff)<<16 | (uint32(sum[o+2])&0xff)<<8 | (uint32(sum[o+3]) & 0xff)
		want := fmt.Sprintf("%06d", bin%1000000)
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true
		}
	}
	return false
}
