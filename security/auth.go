package security

import (
	"crypto/tls"
)

// requiresClientCert reports whether the ClientAuthType requires a client
// certificate to be provided.
func requiresClientCert(c tls.ClientAuthType) bool {
	switch c {
	case tls.RequireAnyClientCert, tls.RequireAndVerifyClientCert:
		return true
	default:
		return false
	}
}
