package security

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
)

// TLS handshake message types.
const (
	MessageTypeHelloRequest        uint8 = 0
	MessageTypeClientHello         uint8 = 1
	MessageTypeServerHello         uint8 = 2
	MessageTypeNewSessionTicket    uint8 = 4
	MessageTypeEndOfEarlyData      uint8 = 5
	MessageTypeEncryptedExtensions uint8 = 8
	MessageTypeCertificate         uint8 = 11
	MessageTypeServerKeyExchange   uint8 = 12
	MessageTypeCertificateRequest  uint8 = 13
	MessageTypeServerHelloDone     uint8 = 14
	MessageTypeCertificateVerify   uint8 = 15
	MessageTypeClientKeyExchange   uint8 = 16
	MessageTypeFinished            uint8 = 20
	MessageTypeCertificateStatus   uint8 = 22
	MessageTypeKeyUpdate           uint8 = 24
	MessageTypeMessageHash         uint8 = 254 // synthetic message
)

// TLS compression types.
const (
	compressionNone uint8 = 0
)

// TLS signaling cipher suite values
const (
	scsvRenegotiation uint16 = 0x00ff
)

// TLS 1.3 Key Share. See RFC 8446, Section 4.2.8.
type keyShare struct {
	group tls.CurveID
	data  []byte
}

// TLS 1.3 PSK Key Exchange Modes. See RFC 8446, Section 4.2.9.
const (
	pskModePlain uint8 = 0
	pskModeDHE   uint8 = 1
)

// TLS 1.3 PSK Identity. Can be a Session Ticket, or a reference to a saved
// session. See RFC 8446, Section 4.2.11.
type pskIdentity struct {
	label               []byte
	obfuscatedTicketAge uint32
}

// TLS Elliptic Curve Point Formats
// https://www.iana.org/assignments/tls-parameters/tls-parameters.xml#tls-parameters-9
const (
	pointFormatUncompressed uint8 = 0
)

// TLS CertificateStatusType (RFC 3546)
const (
	statusTypeOCSP uint8 = 1
)

// Certificate types (for certificateRequestMsg)
const (
	certTypeRSASign   = 1
	certTypeECDSASign = 64 // ECDSA or EdDSA keys, see RFC 8422, Section 3.
)

// helloRetryRequestRandom is set as the Random value of a ServerHello
// to signal that the message is actually a HelloRetryRequest.
var helloRetryRequestRandom = []byte{ // See RFC 8446, Section 4.1.3.
	0xCF, 0x21, 0xAD, 0x74, 0xE5, 0x9A, 0x61, 0x11,
	0xBE, 0x1D, 0x8C, 0x02, 0x1E, 0x65, 0xB8, 0x91,
	0xC2, 0xA2, 0x11, 0x16, 0x7A, 0xBB, 0x8C, 0x5E,
	0x07, 0x9E, 0x09, 0xE2, 0xC8, 0xA8, 0x33, 0x9C,
}

const (
	// downgradeCanaryTLS12 or downgradeCanaryTLS11 is embedded in the server
	// random as a downgrade protection if the server would be capable of
	// negotiating a higher version. See RFC 8446, Section 4.1.3.
	downgradeCanaryTLS12 = "DOWNGRD\x01"
	downgradeCanaryTLS11 = "DOWNGRD\x00"
)

type HandshakeResult struct {
}

type Handshake func(ctx context.Context, conn transport.Transport, config *tls.Config) (future async.Future[HandshakeResult])
