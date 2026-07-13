package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	authCookieName = "fake_agent_session"
	hashIterations = 120000
	sessionTTL     = 30 * 24 * time.Hour
)

func hashPassword(password string) (string, error) {
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", err
	}
	sum := pbkdf2SHA256([]byte(password), salt[:], hashIterations, 32)
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s",
		hashIterations,
		base64.RawURLEncoding.EncodeToString(salt[:]),
		base64.RawURLEncoding.EncodeToString(sum),
	), nil
}

func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2SHA256([]byte(password), salt, iterations, len(want))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hLen := sha256.Size
	numBlocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, numBlocks*hLen)
	var block [4]byte
	for i := 1; i <= numBlocks; i++ {
		block[0] = byte(i >> 24)
		block[1] = byte(i >> 16)
		block[2] = byte(i >> 8)
		block[3] = byte(i)

		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write(block[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for j := 1; j < iterations; j++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for k := range t {
				t[k] ^= u[k]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func newSessionCookie(passwordHash string, secure bool) (*http.Cookie, error) {
	expires := time.Now().Add(sessionTTL).Unix()
	payload := strconv.FormatInt(expires, 10)
	sig := signSession(payload, passwordHash)
	return &http.Cookie{
		Name:     authCookieName,
		Value:    payload + "." + sig,
		Path:     "/",
		Expires:  time.Unix(expires, 0),
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}, nil
}

func clearSessionCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

func validSessionCookie(r *http.Request, passwordHash string) bool {
	cookie, err := r.Cookie(authCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	want := signSession(parts[0], passwordHash)
	return subtle.ConstantTimeCompare([]byte(parts[1]), []byte(want)) == 1
}

func signSession(payload, passwordHash string) string {
	mac := hmac.New(sha256.New, []byte(passwordHash))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func randomPassword() string {
	var b [18]byte
	if _, err := rand.Read(b[:]); err == nil {
		return base64.RawURLEncoding.EncodeToString(b[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func validateNewPassword(password string) error {
	if strings.TrimSpace(password) == "" {
		return errors.New("password is required")
	}
	if len(password) < 12 {
		return errors.New("password must be at least 12 characters")
	}
	return nil
}
