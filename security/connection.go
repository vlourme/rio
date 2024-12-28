package security

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"unsafe"
)

type ConnectionState struct {
	Version                     uint16
	HandshakeComplete           bool
	DidResume                   bool
	CipherSuite                 uint16
	NegotiatedProtocol          string
	NegotiatedProtocolIsMutual  bool
	ServerName                  string
	PeerCertificates            []*x509.Certificate
	VerifiedChains              [][]*x509.Certificate
	SignedCertificateTimestamps [][]byte
	OCSPResponse                []byte
	TLSUnique                   []byte
	ECHAccepted                 bool
	ekm                         func(label string, context []byte, length int) ([]byte, error)
}

func (cs *ConnectionState) ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error) {
	return cs.ekm(label, context, length)
}

func (cs *ConnectionState) SetExportKeyingMaterial(config *tls.Config, version uint16, extMasterSecret bool, defaultEKM ExportKeyingMaterial) {
	if config.Renegotiation != tls.RenegotiateNever {
		cs.ekm = noEKMBecauseRenegotiation
	} else if version != tls.VersionTLS13 && !extMasterSecret {
		cs.ekm = func(label string, context []byte, length int) ([]byte, error) {
			return noEKMBecauseNoEMS(label, context, length)
		}
	} else {
		cs.ekm = defaultEKM
	}
}

func (cs *ConnectionState) AsTLSConnectionState() tls.ConnectionState {
	state := (*tls.ConnectionState)(unsafe.Pointer(cs))
	return *state
}

type permanentError struct {
	err net.Error
}

func (e *permanentError) Error() string   { return e.err.Error() }
func (e *permanentError) Unwrap() error   { return e.err }
func (e *permanentError) Timeout() bool   { return e.err.Timeout() }
func (e *permanentError) Temporary() bool { return false }
