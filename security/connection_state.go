package security

import (
	"crypto/tls"
	"crypto/x509"
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

func (conn *connection) ConnectionState() ConnectionState {
	var state ConnectionState
	state.HandshakeComplete = conn.handshakeComplete.Load()
	state.Version = conn.vers
	state.NegotiatedProtocol = conn.clientProtocol
	state.DidResume = conn.didResume
	// c.curveID is not set on TLS 1.0–1.2 resumptions. Fix that before exposing it.
	state.NegotiatedProtocolIsMutual = true
	state.ServerName = conn.serverName
	state.CipherSuite = conn.cipherSuite
	state.PeerCertificates = conn.peerCertificates
	state.VerifiedChains = conn.verifiedChains
	state.SignedCertificateTimestamps = conn.scts
	state.OCSPResponse = conn.ocspResponse
	if (!conn.didResume || conn.extMasterSecret) && conn.vers != tls.VersionTLS13 {
		if conn.clientFinishedIsFirst {
			state.TLSUnique = conn.clientFinished[:]
		} else {
			state.TLSUnique = conn.serverFinished[:]
		}
	}
	if conn.config.Renegotiation != tls.RenegotiateNever {
		state.ekm = noEKMBecauseRenegotiation
	} else if conn.vers != tls.VersionTLS13 && !conn.extMasterSecret {
		state.ekm = func(label string, context []byte, length int) ([]byte, error) {
			return noEKMBecauseNoEMS(label, context, length)
		}
	} else {
		state.ekm = conn.ekm
	}
	state.ECHAccepted = conn.echAccepted
	return state
}
