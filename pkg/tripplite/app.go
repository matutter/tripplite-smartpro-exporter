package tripplite

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
)

type App interface {
	HMACEnabled() bool
	GetSecret() []byte
	SetChangeId(string)
	IsStale() bool
}

func ValidateHMAC(app App, msg []byte, expectedMACB64 string) (bool, error) {
	if !app.HMACEnabled() {
		return true, nil
	}

	if len(msg) == 0 || len(expectedMACB64) == 0 {
		return false, fmt.Errorf("empty raw message or epxected HMAC")
	}

	secret := app.GetSecret()
	if len(secret) == 0 {
		return false, fmt.Errorf("HMAC secret value is empty")
	}

	mac := hmac.New(sha256.New, secret).Sum(msg)
	expectedMAC, err := base64.RawStdEncoding.DecodeString(expectedMACB64)
	if err != nil {
		return false, err
	}
	return hmac.Equal(mac, expectedMAC), nil
}

func SetHMACHeaders(app App, msg []byte, w http.ResponseWriter) {
	secret := app.GetSecret()
	if len(secret) > 0 {
		mac := hmac.New(sha256.New, secret).Sum(msg)
		w.Header().Add(HTTP_CONTENT_HASH_HEADER, base64.RawStdEncoding.EncodeToString(mac))
	}
}

type PublicConfig struct {
	Delay   string         `json:"delay"`
	Scripts []PublicScript `json:"scripts"`
}
