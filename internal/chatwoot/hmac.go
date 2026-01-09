package chatwoot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// IdentifierHash returns the hex-encoded HMAC-SHA256 of the identifier.
func IdentifierHash(identifier, hmacToken string) string {
	mac := hmac.New(sha256.New, []byte(hmacToken))
	mac.Write([]byte(identifier))
	return hex.EncodeToString(mac.Sum(nil))
}
