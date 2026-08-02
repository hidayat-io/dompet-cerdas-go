package crypto

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"math/big"
)

const inviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// SecureToken returns a hex-encoded cryptographically random token of the given
// byte length (default production value is 32 → 64 hex chars).
func SecureToken(byteLen int) (string, error) {
	if byteLen <= 0 {
		byteLen = 32
	}
	buf := make([]byte, byteLen)
	if _, err := cryptorand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// InviteCode returns an 8-character invite code from the legacy alphabet
// (no I/O/0/1 to reduce transcription errors).
func InviteCode() (string, error) {
	const length = 8
	out := make([]byte, length)
	max := big.NewInt(int64(len(inviteAlphabet)))
	for i := 0; i < length; i++ {
		n, err := cryptorand.Int(cryptorand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = inviteAlphabet[n.Int64()]
	}
	return string(out), nil
}
