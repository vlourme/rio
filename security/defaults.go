package security

import (
	"crypto/tls"
)

// Defaults are collected in this file to allow distributions to more easily patch
// them to apply local policies.

//var tlskyber = godebug.New("tlskyber")
//
//func defaultCurvePreferences() []CurveID {
//	if tlskyber.Value() == "0" {
//		return []CurveID{X25519, CurveP256, CurveP384, CurveP521}
//	}
//	// For now, x25519Kyber768Draft00 must always be followed by X25519.
//	return []CurveID{x25519Kyber768Draft00, X25519, CurveP256, CurveP384, CurveP521}
//}

func defaultCurvePreferences() []tls.CurveID {
	return []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384, tls.CurveP521}
}

// defaultSupportedSignatureAlgorithms contains the signature and hash algorithms that
// the code advertises as supported in a TLS 1.2+ ClientHello and in a TLS 1.2+
// CertificateRequest. The two fields are merged to match with TLS 1.3.
// Note that in TLS 1.2, the ECDSA algorithms are not constrained to P-256, etc.
var defaultSupportedSignatureAlgorithms = []tls.SignatureScheme{
	tls.PSSWithSHA256,
	tls.ECDSAWithP256AndSHA256,
	tls.Ed25519,
	tls.PSSWithSHA384,
	tls.PSSWithSHA512,
	tls.PKCS1WithSHA256,
	tls.PKCS1WithSHA384,
	tls.PKCS1WithSHA512,
	tls.ECDSAWithP384AndSHA384,
	tls.ECDSAWithP521AndSHA512,
	tls.PKCS1WithSHA1,
	tls.ECDSAWithSHA1,
}

//var tlsrsakex = godebug.New("tlsrsakex")
//var tls3des = godebug.New("tls3des")
//
//func defaultCipherSuites() []uint16 {
//	suites := slices.Clone(cipherSuitesPreferenceOrder)
//	return slices.DeleteFunc(suites, func(c uint16) bool {
//		return disabledCipherSuites[c] ||
//			tlsrsakex.Value() != "1" && rsaKexCiphers[c] ||
//			tls3des.Value() != "1" && tdesCiphers[c]
//	})
//}

var (
	defaultCipherSuitesLen = len(cipherSuitesPreferenceOrder) - len(disabledCipherSuites)
)

func defaultCipherSuites() []uint16 {
	return cipherSuitesPreferenceOrder[:defaultCipherSuitesLen]
}

// defaultCipherSuitesTLS13 is also the preference order, since there are no
// disabled by default TLS 1.3 cipher suites. The same AES vs ChaCha20 logic as
// cipherSuitesPreferenceOrder applies.
//
// defaultCipherSuitesTLS13 should be an internal detail,
// but widely used packages access it using linkname.
var defaultCipherSuitesTLS13 = []uint16{
	TLS_AES_128_GCM_SHA256,
	TLS_AES_256_GCM_SHA384,
	TLS_CHACHA20_POLY1305_SHA256,
}

// defaultCipherSuitesTLS13NoAES should be an internal detail,
// but widely used packages access it using linkname.
var defaultCipherSuitesTLS13NoAES = []uint16{
	TLS_CHACHA20_POLY1305_SHA256,
	TLS_AES_128_GCM_SHA256,
	TLS_AES_256_GCM_SHA384,
}

var defaultSupportedVersionsFIPS = []uint16{
	VersionTLS12,
}

// defaultCurvePreferencesFIPS are the FIPS-allowed curves,
// in preference order (most preferable first).
var defaultCurvePreferencesFIPS = []tls.CurveID{tls.CurveP256, tls.CurveP384, tls.CurveP521}

// defaultSupportedSignatureAlgorithmsFIPS currently are a subset of
// defaultSupportedSignatureAlgorithms without Ed25519 and SHA-1.
var defaultSupportedSignatureAlgorithmsFIPS = []tls.SignatureScheme{
	tls.PSSWithSHA256,
	tls.PSSWithSHA384,
	tls.PSSWithSHA512,
	tls.PKCS1WithSHA256,
	tls.ECDSAWithP256AndSHA256,
	tls.PKCS1WithSHA384,
	tls.ECDSAWithP384AndSHA384,
	tls.PKCS1WithSHA512,
	tls.ECDSAWithP521AndSHA512,
}

// defaultCipherSuitesFIPS are the FIPS-allowed cipher suites.
var defaultCipherSuitesFIPS = []uint16{
	TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	TLS_RSA_WITH_AES_128_GCM_SHA256,
	TLS_RSA_WITH_AES_256_GCM_SHA384,
}

// defaultCipherSuitesTLS13FIPS are the FIPS-allowed cipher suites for TLS 1.3.
var defaultCipherSuitesTLS13FIPS = []uint16{
	TLS_AES_128_GCM_SHA256,
	TLS_AES_256_GCM_SHA384,
}
