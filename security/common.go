package security

import (
	"crypto/tls"
	"fmt"
	_ "unsafe" // for linkname
)

// roleClient and roleServer are meant to call supportedVersions and parents
// with more readability at the callsite.
const roleClient = true
const roleServer = false

func unexpectedMessageError(wanted, got any) error {
	return fmt.Errorf("tls: received unexpected handshake message of type %T when waiting for %T", got, wanted)
}

// supportedSignatureAlgorithms returns the supported signature algorithms.
func supportedSignatureAlgorithms() []tls.SignatureScheme {
	if !needFIPS() {
		return defaultSupportedSignatureAlgorithms
	}
	return defaultSupportedSignatureAlgorithmsFIPS
}

func isSupportedSignatureAlgorithm(sigAlg tls.SignatureScheme, supportedSignatureAlgorithms []tls.SignatureScheme) bool {
	for _, s := range supportedSignatureAlgorithms {
		if s == sigAlg {
			return true
		}
	}
	return false
}
