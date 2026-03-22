package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FileTokenTTL is the default TTL for signed file tokens.
const FileTokenTTL = 1 * time.Hour

// SignFileToken creates a short-lived HMAC token for file access.
// Token format: {base64url_hmac_16bytes}.{unix_expiry} (~40 chars).
// The path is bound into the signature so tokens can't be reused for other files.
func SignFileToken(path, secret string, ttl time.Duration) string {
	expiry := time.Now().Add(ttl).Unix()
	sig := fileTokenHMAC(path, secret, expiry)
	return fmt.Sprintf("%s.%d", sig, expiry)
}

// VerifyFileToken validates a signed file token against a path and secret.
// Returns true if the HMAC matches and the token has not expired.
func VerifyFileToken(token, path, secret string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expiry, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	expected := fileTokenHMAC(path, secret, expiry)
	return hmac.Equal([]byte(parts[0]), []byte(expected))
}

// fileTokenHMAC computes the HMAC signature component.
func fileTokenHMAC(path, secret string, expiry int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%s:%d", path, expiry)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}
