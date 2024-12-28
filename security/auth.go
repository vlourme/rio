package security

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"fmt"
	"hash"
	"io"
)

// ConnectionState records basic TLS details about the connection.

// ClientAuthType declares the policy the server will follow for
// TLS Client Authentication.
type ClientAuthType int

const (
	// NoClientCert indicates that no client certificate should be requested
	// during the handshake, and if any certificates are sent they will not
	// be verified.
	NoClientCert ClientAuthType = iota
	// RequestClientCert indicates that a client certificate should be requested
	// during the handshake, but does not require that the client send any
	// certificates.
	RequestClientCert
	// RequireAnyClientCert indicates that a client certificate should be requested
	// during the handshake, and that at least one certificate is required to be
	// sent by the client, but that certificate is not required to be valid.
	RequireAnyClientCert
	// VerifyClientCertIfGiven indicates that a client certificate should be requested
	// during the handshake, but does not require that the client sends a
	// certificate. If the client does send a certificate it is required to be
	// valid.
	VerifyClientCertIfGiven
	// RequireAndVerifyClientCert indicates that a client certificate should be requested
	// during the handshake, and that at least one valid certificate is required
	// to be sent by the client.
	RequireAndVerifyClientCert
)

// requiresClientCert reports whether the ClientAuthType requires a client
// certificate to be provided.
func requiresClientCert(c ClientAuthType) bool {
	switch c {
	case RequireAnyClientCert, RequireAndVerifyClientCert:
		return true
	default:
		return false
	}
}

// Signature algorithms (for internal signaling use). Starting at 225 to avoid overlap with
// TLS 1.2 codepoints (RFC 5246, Appendix A.4.1), with which these have nothing to do.
const (
	signaturePKCS1v15 uint8 = iota + 225
	signatureRSAPSS
	signatureECDSA
	signatureEd25519
)

// directSigning is a standard Hash value that signals that no pre-hashing
// should be performed, and that the input should be signed directly. It is the
// hash function associated with the Ed25519 signature scheme.
var directSigning crypto.Hash = 0

// verifyHandshakeSignature verifies a signature against pre-hashed
// (if required) handshake contents.
func verifyHandshakeSignature(sigType uint8, pubkey crypto.PublicKey, hashFunc crypto.Hash, signed, sig []byte) error {
	switch sigType {
	case signatureECDSA:
		pubKey, ok := pubkey.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected an ECDSA public key, got %T", pubkey)
		}
		if !ecdsa.VerifyASN1(pubKey, signed, sig) {
			return errors.New("ECDSA verification failure")
		}
	case signatureEd25519:
		pubKey, ok := pubkey.(ed25519.PublicKey)
		if !ok {
			return fmt.Errorf("expected an Ed25519 public key, got %T", pubkey)
		}
		if !ed25519.Verify(pubKey, signed, sig) {
			return errors.New("Ed25519 verification failure")
		}
	case signaturePKCS1v15:
		pubKey, ok := pubkey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected an RSA public key, got %T", pubkey)
		}
		if err := rsa.VerifyPKCS1v15(pubKey, hashFunc, signed, sig); err != nil {
			return err
		}
	case signatureRSAPSS:
		pubKey, ok := pubkey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("expected an RSA public key, got %T", pubkey)
		}
		signOpts := &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash}
		if err := rsa.VerifyPSS(pubKey, hashFunc, signed, sig, signOpts); err != nil {
			return err
		}
	default:
		return errors.New("internal error: unknown signature type")
	}
	return nil
}

const (
	serverSignatureContext = "TLS 1.3, server CertificateVerify\x00"
	clientSignatureContext = "TLS 1.3, client CertificateVerify\x00"
)

var signaturePadding = []byte{
	0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
	0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
	0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
	0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
	0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
	0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
	0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
	0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20,
}

// signedMessage returns the pre-hashed (if necessary) message to be signed by
// certificate keys in TLS 1.3. See RFC 8446, Section 4.4.3.
func signedMessage(sigHash crypto.Hash, context string, transcript hash.Hash) []byte {
	if sigHash == directSigning {
		b := &bytes.Buffer{}
		b.Write(signaturePadding)
		io.WriteString(b, context)
		b.Write(transcript.Sum(nil))
		return b.Bytes()
	}
	h := sigHash.New()
	h.Write(signaturePadding)
	io.WriteString(h, context)
	h.Write(transcript.Sum(nil))
	return h.Sum(nil)
}

// typeAndHashFromSignatureScheme returns the corresponding signature type and
// crypto.Hash for a given TLS SignatureScheme.
func typeAndHashFromSignatureScheme(signatureAlgorithm tls.SignatureScheme) (sigType uint8, hash crypto.Hash, err error) {
	switch signatureAlgorithm {
	case tls.PKCS1WithSHA1, tls.PKCS1WithSHA256, tls.PKCS1WithSHA384, tls.PKCS1WithSHA512:
		sigType = signaturePKCS1v15
	case tls.PSSWithSHA256, tls.PSSWithSHA384, tls.PSSWithSHA512:
		sigType = signatureRSAPSS
	case tls.ECDSAWithSHA1, tls.ECDSAWithP256AndSHA256, tls.ECDSAWithP384AndSHA384, tls.ECDSAWithP521AndSHA512:
		sigType = signatureECDSA
	case tls.Ed25519:
		sigType = signatureEd25519
	default:
		return 0, 0, fmt.Errorf("unsupported signature algorithm: %v", signatureAlgorithm)
	}
	switch signatureAlgorithm {
	case tls.PKCS1WithSHA1, tls.ECDSAWithSHA1:
		hash = crypto.SHA1
	case tls.PKCS1WithSHA256, tls.PSSWithSHA256, tls.ECDSAWithP256AndSHA256:
		hash = crypto.SHA256
	case tls.PKCS1WithSHA384, tls.PSSWithSHA384, tls.ECDSAWithP384AndSHA384:
		hash = crypto.SHA384
	case tls.PKCS1WithSHA512, tls.PSSWithSHA512, tls.ECDSAWithP521AndSHA512:
		hash = crypto.SHA512
	case tls.Ed25519:
		hash = directSigning
	default:
		return 0, 0, fmt.Errorf("unsupported signature algorithm: %v", signatureAlgorithm)
	}
	return sigType, hash, nil
}

// legacyTypeAndHashFromPublicKey returns the fixed signature type and crypto.Hash for
// a given public key used with TLS 1.0 and 1.1, before the introduction of
// signature algorithm negotiation.
func legacyTypeAndHashFromPublicKey(pub crypto.PublicKey) (sigType uint8, hash crypto.Hash, err error) {
	switch pub.(type) {
	case *rsa.PublicKey:
		return signaturePKCS1v15, crypto.MD5SHA1, nil
	case *ecdsa.PublicKey:
		return signatureECDSA, crypto.SHA1, nil
	case ed25519.PublicKey:
		// RFC 8422 specifies support for Ed25519 in TLS 1.0 and 1.1,
		// but it requires holding on to a handshake transcript to do a
		// full signature, and not even OpenSSL bothers with the
		// complexity, so we can't even test it properly.
		return 0, 0, fmt.Errorf("tls: Ed25519 public keys are not supported before TLS 1.2")
	default:
		return 0, 0, fmt.Errorf("tls: unsupported public key: %T", pub)
	}
}

var rsaSignatureSchemes = []struct {
	scheme          tls.SignatureScheme
	minModulusBytes int
	maxVersion      uint16
}{
	// RSA-PSS is used with PSSSaltLengthEqualsHash, and requires
	//    emLen >= hLen + sLen + 2
	{tls.PSSWithSHA256, crypto.SHA256.Size()*2 + 2, VersionTLS13},
	{tls.PSSWithSHA384, crypto.SHA384.Size()*2 + 2, VersionTLS13},
	{tls.PSSWithSHA512, crypto.SHA512.Size()*2 + 2, VersionTLS13},
	// PKCS #1 v1.5 uses prefixes from hashPrefixes in crypto/rsa, and requires
	//    emLen >= len(prefix) + hLen + 11
	// TLS 1.3 dropped support for PKCS #1 v1.5 in favor of RSA-PSS.
	{tls.PKCS1WithSHA256, 19 + crypto.SHA256.Size() + 11, VersionTLS12},
	{tls.PKCS1WithSHA384, 19 + crypto.SHA384.Size() + 11, VersionTLS12},
	{tls.PKCS1WithSHA512, 19 + crypto.SHA512.Size() + 11, VersionTLS12},
	{tls.PKCS1WithSHA1, 15 + crypto.SHA1.Size() + 11, VersionTLS12},
}

// signatureSchemesForCertificate returns the list of supported SignatureSchemes
// for a given certificate, based on the public key and the protocol version,
// and optionally filtered by its explicit SupportedSignatureAlgorithms.
//
// This function must be kept in sync with supportedSignatureAlgorithms.
// FIPS filtering is applied in the caller, selectSignatureScheme.
func signatureSchemesForCertificate(version uint16, cert *tls.Certificate) []tls.SignatureScheme {
	priv, ok := cert.PrivateKey.(crypto.Signer)
	if !ok {
		return nil
	}

	var sigAlgs []tls.SignatureScheme
	switch pub := priv.Public().(type) {
	case *ecdsa.PublicKey:
		if version != VersionTLS13 {
			// In TLS 1.2 and earlier, ECDSA algorithms are not
			// constrained to a single curve.
			sigAlgs = []tls.SignatureScheme{
				tls.ECDSAWithP256AndSHA256,
				tls.ECDSAWithP384AndSHA384,
				tls.ECDSAWithP521AndSHA512,
				tls.ECDSAWithSHA1,
			}
			break
		}
		switch pub.Curve {
		case elliptic.P256():
			sigAlgs = []tls.SignatureScheme{tls.ECDSAWithP256AndSHA256}
		case elliptic.P384():
			sigAlgs = []tls.SignatureScheme{tls.ECDSAWithP384AndSHA384}
		case elliptic.P521():
			sigAlgs = []tls.SignatureScheme{tls.ECDSAWithP521AndSHA512}
		default:
			return nil
		}
	case *rsa.PublicKey:
		size := pub.Size()
		sigAlgs = make([]tls.SignatureScheme, 0, len(rsaSignatureSchemes))
		for _, candidate := range rsaSignatureSchemes {
			if size >= candidate.minModulusBytes && version <= candidate.maxVersion {
				sigAlgs = append(sigAlgs, candidate.scheme)
			}
		}
	case ed25519.PublicKey:
		sigAlgs = []tls.SignatureScheme{tls.Ed25519}
	default:
		return nil
	}

	if cert.SupportedSignatureAlgorithms != nil {
		var filteredSigAlgs []tls.SignatureScheme
		for _, sigAlg := range sigAlgs {
			if isSupportedSignatureAlgorithm(sigAlg, cert.SupportedSignatureAlgorithms) {
				filteredSigAlgs = append(filteredSigAlgs, sigAlg)
			}
		}
		return filteredSigAlgs
	}
	return sigAlgs
}

// selectSignatureScheme picks a SignatureScheme from the peer's preference list
// that works with the selected certificate. It's only called for protocol
// versions that support signature algorithms, so TLS 1.2 and 1.3.
func selectSignatureScheme(vers uint16, c *tls.Certificate, peerAlgs []tls.SignatureScheme) (tls.SignatureScheme, error) {
	supportedAlgs := signatureSchemesForCertificate(vers, c)
	if len(supportedAlgs) == 0 {
		return 0, unsupportedCertificateError(c)
	}
	if len(peerAlgs) == 0 && vers == VersionTLS12 {
		// For TLS 1.2, if the client didn't send signature_algorithms then we
		// can assume that it supports SHA1. See RFC 5246, Section 7.4.1.4.1.
		peerAlgs = []tls.SignatureScheme{tls.PKCS1WithSHA1, tls.ECDSAWithSHA1}
	}
	// Pick signature scheme in the peer's preference order, as our
	// preference order is not configurable.
	for _, preferredAlg := range peerAlgs {
		if needFIPS() && !isSupportedSignatureAlgorithm(preferredAlg, defaultSupportedSignatureAlgorithmsFIPS) {
			continue
		}
		if isSupportedSignatureAlgorithm(preferredAlg, supportedAlgs) {
			return preferredAlg, nil
		}
	}
	return 0, errors.New("tls: peer doesn't support any of the certificate's signature algorithms")
}

// unsupportedCertificateError returns a helpful error for certificates with
// an unsupported private key.
func unsupportedCertificateError(cert *tls.Certificate) error {
	switch cert.PrivateKey.(type) {
	case rsa.PrivateKey, ecdsa.PrivateKey:
		return fmt.Errorf("tls: unsupported certificate: private key is %T, expected *%T",
			cert.PrivateKey, cert.PrivateKey)
	case *ed25519.PrivateKey:
		return fmt.Errorf("tls: unsupported certificate: private key is *ed25519.PrivateKey, expected ed25519.PrivateKey")
	}

	signer, ok := cert.PrivateKey.(crypto.Signer)
	if !ok {
		return fmt.Errorf("tls: certificate private key (%T) does not implement crypto.Signer",
			cert.PrivateKey)
	}

	switch pub := signer.Public().(type) {
	case *ecdsa.PublicKey:
		switch pub.Curve {
		case elliptic.P256():
		case elliptic.P384():
		case elliptic.P521():
		default:
			return fmt.Errorf("tls: unsupported certificate curve (%s)", pub.Curve.Params().Name)
		}
	case *rsa.PublicKey:
		return fmt.Errorf("tls: certificate RSA key size too small for supported signature algorithms")
	case ed25519.PublicKey:
	default:
		return fmt.Errorf("tls: unsupported certificate key (%T)", pub)
	}

	if cert.SupportedSignatureAlgorithms != nil {
		return fmt.Errorf("tls: peer doesn't support the certificate custom signature algorithms")
	}

	return fmt.Errorf("tls: internal error: unsupported key (%T)", cert.PrivateKey)
}
