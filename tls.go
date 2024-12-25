package rio

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/brickingsoft/rio/security"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync/atomic"
	"time"
)

type TLSConnectionBuilder func(conn Connection, config *tls.Config) TLSConnection

type TLSConnection interface {
	Connection
	ConnectionState() tls.ConnectionState
	OCSPResponse() []byte
	VerifyHostname(host string) error
	CloseWrite() (future async.Future[async.Void])
	Handshake() (future async.Future[async.Void])
}

func TLSClient(conn Connection, config *tls.Config) TLSConnection {
	c := &tlsConnection{
		inner:    conn,
		config:   config,
		isClient: true,
	}
	c.handshakeFn = security.ClientHandshake
	return c
}

func TLSServer(conn Connection, config *tls.Config) TLSConnection {
	c := &tlsConnection{
		inner:  conn,
		config: config,
	}
	c.handshakeFn = security.ServerHandshake
	return c
}

type tlsConnection struct {
	inner    Connection
	config   *tls.Config
	isClient bool

	handshakeFn       security.Handshake
	handshakeComplete atomic.Bool

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
	activeCertHandles []*security.ActiveCert
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
	// todo use tls.Config.EncryptTicket or DecryptTicket
	// ticketKeys 来自 c.config ticketKeys(configForClient)
	// configForClient 来自 c.config.GetConfigForClient，一般 GetConfigForClient 为空
	// ticketKeys []ticketKey

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

func (conn *tlsConnection) Read() (future async.Future[transport.Inbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Write(b []byte) (future async.Future[int]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Close() (future async.Future[async.Void]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Sendfile(file string) (future async.Future[int]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) ConnectionState() tls.ConnectionState {
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

func (conn *tlsConnection) OCSPResponse() []byte {

	return conn.ocspResponse
}

func (conn *tlsConnection) VerifyHostname(host string) error {
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

func (conn *tlsConnection) CloseWrite() (future async.Future[async.Void]) {
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

	security.CloseNotify(ctx, conn).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
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

func (conn *tlsConnection) Handshake() (future async.Future[async.Void]) {
	ctx := conn.Context()
	if conn.handshakeComplete.Load() {
		future = async.SucceedImmediately[async.Void](ctx, async.Void{})
		return
	}
	promise, promiseErr := async.Make[async.Void](ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[async.Void](ctx, promiseErr)
		return
	}
	future = promise.Future()

	conn.handshakeFn(ctx, conn.inner, conn.config).OnComplete(func(ctx context.Context, entry security.HandshakeResult, cause error) {
		if cause != nil {
			promise.Fail(cause)
			return
		}
		// todo handle handshake result
		conn.handshakeComplete.Store(true)
		promise.Succeed(async.Void{})
		return
	})

	return
}

func (conn *tlsConnection) Context() context.Context {
	return conn.inner.Context()
}

func (conn *tlsConnection) ConfigContext(config func(ctx context.Context) context.Context) {
	conn.inner.ConfigContext(config)
}

func (conn *tlsConnection) Fd() int {
	return conn.inner.Fd()
}

func (conn *tlsConnection) LocalAddr() net.Addr {
	return conn.inner.LocalAddr()
}

func (conn *tlsConnection) RemoteAddr() net.Addr {
	return conn.inner.RemoteAddr()
}

func (conn *tlsConnection) SetReadTimeout(d time.Duration) error {
	return conn.inner.SetReadTimeout(d)
}

func (conn *tlsConnection) SetWriteTimeout(d time.Duration) error {
	return conn.inner.SetWriteTimeout(d)
}

func (conn *tlsConnection) SetReadBuffer(n int) error {
	return conn.inner.SetReadBuffer(n)
}

func (conn *tlsConnection) SetWriteBuffer(n int) error {
	return conn.inner.SetWriteBuffer(n)
}

func (conn *tlsConnection) SetInboundBuffer(n int) {
	conn.inner.SetInboundBuffer(n)
}
