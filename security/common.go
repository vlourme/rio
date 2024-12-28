package security

import (
	"crypto/tls"
	"fmt"
	"time"
	_ "unsafe" // for linkname
)

// SignatureScheme identifies a signature algorithm supported by TLS. See
// RFC 8446, Section 4.2.3.

// RenegotiationSupport enumerates the different levels of support for TLS
// renegotiation. TLS renegotiation is the act of performing subsequent
// handshakes on a connection after the first. This significantly complicates
// the state machine and has been the source of numerous, subtle security
// issues. Initiating a renegotiation is not supported, but support for
// accepting renegotiation requests may be enabled.
//
// Even when enabled, the server may not change its identity between handshakes
// (i.e. the leaf certificate must be the same). Additionally, concurrent
// handshake and application data flow is not permitted so renegotiation can
// only be used with protocols that synchronise with the renegotiation, such as
// HTTPS.
//
// Renegotiation is not defined in TLS 1.3.
type RenegotiationSupport int

const (
	// RenegotiateNever disables renegotiation.
	RenegotiateNever RenegotiationSupport = iota

	// RenegotiateOnceAsClient allows a remote server to request
	// renegotiation once per connection.
	RenegotiateOnceAsClient

	// RenegotiateFreelyAsClient allows a remote server to repeatedly
	// request renegotiation.
	RenegotiateFreelyAsClient
)

const (
	// ticketKeyLifetime is how long a ticket key remains valid and can be used to
	// resume a client connection.
	ticketKeyLifetime = 7 * 24 * time.Hour // 7 days

	// ticketKeyRotation is how often the server should rotate the session ticket key
	// that is used for new tickets.
	ticketKeyRotation = 24 * time.Hour
)

// ticketKey is the internal representation of a session ticket key.
type ticketKey struct {
	aesKey  [16]byte
	hmacKey [16]byte
	// created is the time at which this ticket key was created. See Config.ticketKeys.
	created time.Time
}

var supportedVersions = []uint16{
	VersionTLS13,
	VersionTLS12,
	VersionTLS11,
	VersionTLS10,
}

// roleClient and roleServer are meant to call supportedVersions and parents
// with more readability at the callsite.
const roleClient = true
const roleServer = false

// supportedVersionsFromMax returns a list of supported versions derived from a
// legacy maximum version value. Note that only versions supported by this
// library are returned. Any newer peer will use supportedVersions anyway.
func supportedVersionsFromMax(maxVersion uint16) []uint16 {
	versions := make([]uint16, 0, len(supportedVersions))
	for _, v := range supportedVersions {
		if v > maxVersion {
			continue
		}
		versions = append(versions, v)
	}
	return versions
}

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
