package security

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"strconv"
	"sync/atomic"
	"unsafe"
)

type ConnectionBuilder interface {
	Client(ts transport.Connection) Connection
	Server(ts transport.Connection) Connection
}

func NewConnectionBuilder(config *tls.Config) ConnectionBuilder {
	return &defaultConnectionBuilder{
		config: config,
	}
}

type defaultConnectionBuilder struct {
	config *tls.Config
}

func (builder *defaultConnectionBuilder) Client(ts transport.Connection) Connection {
	c := &connection{
		Connection:          ts,
		config:              builder.config,
		isClient:            true,
		handshakeBarrier:    async.NewBarrier[async.Void](),
		handshakeBarrierKey: strconv.Itoa(ts.Fd()),
	}
	c.handshakeFn = ClientHandshake
	return c
}

func (builder *defaultConnectionBuilder) Server(ts transport.Connection) Connection {
	c := &connection{
		Connection:          ts,
		config:              builder.config,
		handshakeBarrier:    async.NewBarrier[async.Void](),
		handshakeBarrierKey: strconv.Itoa(ts.Fd()),
	}
	c.handshakeFn = ServerHandshake
	return c
}

type Connection interface {
	transport.Connection
	ConnectionState() tls.ConnectionState
	OCSPResponse() []byte
	VerifyHostname(host string) error
	CloseWrite() (future async.Future[async.Void])
	Handshake() (future async.Future[async.Void])
}

type connection struct {
	transport.Connection

	config   *tls.Config
	isClient bool

	handshakeFn         Handshake
	handshakeComplete   atomic.Bool
	handshakeErr        error
	handshakeBarrier    async.Barrier[async.Void]
	handshakeBarrierKey string

	vers     uint16 // TLS version
	haveVers bool   // version has been negotiated

	handshakes       int
	extMasterSecret  bool
	didResume        bool // whether this connection was a session resumption
	didHRR           bool // whether a HelloRetryRequest was sent/received
	cipherSuite      uint16
	curveID          tls.CurveID
	ocspResponse     []byte   // stapled OCSP response
	scts             [][]byte // signed certificate timestamps from server
	peerCertificates []*x509.Certificate
	// activeCertHandles contains the cache handles to certificates in
	// peerCertificates that are used to track active references.
	// 来自 verifyServerCertificate
	activeCertHandles []*x509.Certificate
	// verifiedChains contains the certificate chains that we built, as
	// opposed to the ones presented by the server.
	verifiedChains [][]*x509.Certificate
	// serverName contains the server name indicated by the client, if any.
	serverName string
	// secureRenegotiation is true if the server echoed the secure
	// renegotiation extension. (This is meaningless as a server because
	// renegotiation is not supported in that case.)
	secureRenegotiation bool
	// ekm is a closure for exporting keying material.
	ekm func(label string, context []byte, length int) ([]byte, error)
	// resumptionSecret is the resumption_master_secret for handling
	// or sending NewSessionTicket messages.
	resumptionSecret []byte
	echAccepted      bool

	// ticketKeys is the set of active session ticket keys for this
	// connection. The first one is used to encrypt new tickets and
	// all are tried to decrypt tickets.
	ticketKeys []ticketKey

	// clientFinishedIsFirst is true if the client sent the first Finished
	// message during the most recent handshake. This is recorded because
	// the first transmitted Finished message is the tls-unique
	// channel-binding value.
	clientFinishedIsFirst bool

	// closeNotifyErr is any error from sending the alertCloseNotify record.
	closeNotifyErr error
	// closeNotifySent is true if the Conn attempted to send an
	// alertCloseNotify record.
	closeNotifySent bool

	// clientFinished and serverFinished contain the Finished message sent
	// by the client or server in the most recent handshake. This is
	// retained to support the renegotiation extension and tls-unique
	// channel-binding.
	clientFinished [12]byte
	serverFinished [12]byte

	// clientProtocol is the negotiated ALPN protocol.
	clientProtocol string

	// retryCount counts the number of consecutive non-advancing records
	// received by Conn.readRecord. That is, records that neither advance the
	// handshake, nor deliver application data. Protected by in.Mutex.
	retryCount int

	// activeCall indicates whether Close has been call in the low bit.
	// the rest of the bits are the number of goroutines in Conn.Write.
	activeCall atomic.Int32

	tmp [16]byte
}

func (conn *connection) Read() (future async.Future[transport.Inbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) Write(b []byte) (future async.Future[int]) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) Close() (future async.Future[async.Void]) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) Sendfile(file string) (future async.Future[int]) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) ConnectionState() tls.ConnectionState {
	var state tls.ConnectionState
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
	// todo handle ekm
	//if conn.config.Renegotiation != tls.RenegotiateNever {
	//	state.ekm = noEKMBecauseRenegotiation
	//} else if conn.vers != tls.VersionTLS13 && !conn.extMasterSecret {
	//	state.ekm = func(label string, context []byte, length int) ([]byte, error) {
	//		if tlsunsafeekm.Value() == "1" {
	//			tlsunsafeekm.IncNonDefault()
	//			return c.ekm(label, context, length)
	//		}
	//		return noEKMBecauseNoEMS(label, context, length)
	//	}
	//} else {
	//	state.ekm = conn.ekm
	//}
	state.ECHAccepted = conn.echAccepted
	return state
}

func (conn *connection) OCSPResponse() []byte {

	return conn.ocspResponse
}

func (conn *connection) VerifyHostname(host string) error {
	if !conn.isClient {
		return errors.New("rio: VerifyHostname called on TLS server connection")
	}
	if !conn.handshakeComplete.Load() {
		return errors.New("rio: handshake has not yet been performed")
	}
	if len(conn.verifiedChains) == 0 {
		return errors.New("rio: handshake did not verify certificate chain")
	}
	return conn.peerCertificates[0].VerifyHostname(host)
}

var errEarlyCloseWrite = errors.New("rio: CloseWrite called before handshake complete")

func (conn *connection) CloseWrite() (future async.Future[async.Void]) {
	ctx := conn.Context()
	if !conn.handshakeComplete.Load() {
		future = async.FailedImmediately[async.Void](ctx, errEarlyCloseWrite)
		return
	}
	if conn.closeNotifySent {
		if conn.closeNotifyErr != nil {
			future = async.FailedImmediately[async.Void](ctx, conn.closeNotifyErr)
			return
		}
		future = async.SucceedImmediately[async.Void](ctx, async.Void{})
		return
	}

	promise, promiseErr := async.Make[async.Void](ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[async.Void](ctx, promiseErr)
		return
	}
	future = promise.Future()

	CloseNotify(ctx, conn).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		conn.closeNotifySent = true
		if cause != nil {
			conn.closeNotifyErr = cause
			promise.Fail(cause)
		} else {
			promise.Succeed(async.Void{})
		}
		return
	})
	return
}

func (conn *connection) Handshake() (future async.Future[async.Void]) {
	ctx := conn.Context()
	if conn.handshakeComplete.Load() {
		if conn.handshakeErr != nil {
			future = async.FailedImmediately[async.Void](ctx, conn.handshakeErr)
		} else {
			future = async.SucceedImmediately[async.Void](ctx, async.Void{})
		}
		return
	}

	future = conn.handshakeBarrier.Do(ctx, conn.handshakeBarrierKey, func(promise async.Promise[async.Void]) {
		conn.handshakeFn(ctx, conn.Connection, conn.config).OnComplete(func(ctx context.Context, entry HandshakeResult, cause error) {
			conn.handshakeComplete.Store(true)
			if cause != nil {
				conn.handshakeErr = cause
				promise.Fail(cause)
				return
			}
			// todo handle handshake result
			promise.Succeed(async.Void{})
			return
		})
	}, async.WithWait())

	return
}

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
