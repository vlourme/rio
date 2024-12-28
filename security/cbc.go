package security

import "crypto/cipher"

// cbcMode is an interface for block ciphers using cipher block chaining.
type cbcMode interface {
	cipher.BlockMode
	SetIV([]byte)
}

func roundUp(a, b int) int {
	return a + (b-a%b)%b
}
