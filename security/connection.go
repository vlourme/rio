package security

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"strconv"
	"sync/atomic"
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
		inbound:             transport.NewInboundBuffer(),
	}
	c.handshakeFn = c.clientHandshake
	return c
}

func (builder *defaultConnectionBuilder) Server(ts transport.Connection) Connection {
	c := &connection{
		Connection:          ts,
		config:              builder.config,
		handshakeBarrier:    async.NewBarrier[async.Void](),
		handshakeBarrierKey: strconv.Itoa(ts.Fd()),
		inbound:             transport.NewInboundBuffer(),
	}
	c.handshakeFn = c.serverHandshake
	return c
}

type Connection interface {
	transport.Connection
	ConnectionState() ConnectionState
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

	// input/output
	in, out halfConnection
	inbound transport.InboundBuffer

	// bytesSent counts the bytes of application data sent.
	// packetsSent counts packets.
	bytesSent   int64
	packetsSent int64

	// retryCount counts the number of consecutive non-advancing records
	// received by Conn.readRecord. That is, records that neither advance the
	// handshake, nor deliver application data. Protected by in.Mutex.
	retryCount int

	// activeCall indicates whether Close has been call in the low bit.
	// the rest of the bits are the number of goroutines in Conn.Write.
	activeCall atomic.Int32

	tmp [16]byte
}

func (conn *connection) OCSPResponse() []byte {

	return conn.ocspResponse
}

func (conn *connection) VerifyHostname(host string) error {
	if !conn.isClient {
		return errors.New("tls: VerifyHostname called on TLS server connection")
	}
	if !conn.handshakeComplete.Load() {
		return errors.New("tls: handshake has not yet been performed")
	}
	if len(conn.verifiedChains) == 0 {
		return errors.New("tls: handshake did not verify certificate chain")
	}
	return conn.peerCertificates[0].VerifyHostname(host)
}
