package security

import (
	"bytes"
	"context"
	"crypto/x509"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type ConnectionBuilder func(ctx context.Context, fd aio.NetFd, config *Config) (conn Connection, err error)

type Connection interface {
	Context() (ctx context.Context)
	ConfigContext(config func(ctx context.Context) context.Context)
	Fd() int
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetReadTimeout(d time.Duration) (err error)
	SetWriteTimeout(d time.Duration) (err error)
	SetReadBuffer(n int) (err error)
	SetWriteBuffer(n int) (err error)
	SetInboundBuffer(n int)
	MultipathTCP() bool
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
	SetKeepAliveConfig(config aio.KeepAliveConfig) (err error)
	Read() (future async.Future[transport.Inbound])
	Write(b []byte) (future async.Future[transport.Outbound])
	Close() (future async.Future[async.Void])
}

type TLSConnection struct {
	ctx                   context.Context
	fd                    aio.NetFd
	rb                    transport.InboundBuffer
	rbs                   int
	isClient              bool
	handshakeFn           func(ctx context.Context) (future async.Future[async.Void])
	isHandshakeComplete   atomic.Bool
	handshakeMutex        sync.Mutex
	handshakeErr          error   // error resulting from handshake
	vers                  uint16  // TLS version
	haveVers              bool    // version has been negotiated
	config                *Config // configuration passed to constructor
	handshakes            int
	extMasterSecret       bool
	didResume             bool // whether this connection was a session resumption
	didHRR                bool // whether a HelloRetryRequest was sent/received
	cipherSuite           uint16
	curveID               CurveID
	ocspResponse          []byte   // stapled OCSP response
	scts                  [][]byte // signed certificate timestamps from server
	peerCertificates      []*x509.Certificate
	activeCertHandles     []*activeCert
	verifiedChains        [][]*x509.Certificate
	serverName            string
	secureRenegotiation   bool
	ekm                   func(label string, context []byte, length int) ([]byte, error)
	resumptionSecret      []byte
	echAccepted           bool
	ticketKeys            []ticketKey
	clientFinishedIsFirst bool
	closeNotifyErr        error
	closeNotifySent       bool
	clientFinished        [12]byte
	serverFinished        [12]byte
	clientProtocol        string
	in, out               halfConn
	rawInput              bytes.Buffer // raw input, starting with a record header
	input                 bytes.Reader // application data waiting to be read, from rawInput.Next
	hand                  bytes.Buffer // handshake data waiting to be read
	buffering             bool         // whether records are buffered in sendBuf
	sendBuf               []byte       // a buffer of records waiting to be sent
	bytesSent             int64
	packetsSent           int64
	retryCount            int
	activeCall            atomic.Int32
	tmp                   [16]byte
}

func (conn *TLSConnection) Context() context.Context {
	return conn.ctx
}

func (conn *TLSConnection) ConfigContext(config func(ctx context.Context) context.Context) {
	if config == nil {
		return
	}
	newCtx := config(conn.ctx)
	if newCtx == nil {
		return
	}
	conn.ctx = newCtx
	return
}

func (conn *TLSConnection) Fd() int {
	return conn.fd.Fd()
}

func (conn *TLSConnection) LocalAddr() (addr net.Addr) {
	addr = conn.fd.LocalAddr()
	return
}

func (conn *TLSConnection) RemoteAddr() (addr net.Addr) {
	addr = conn.fd.RemoteAddr()
	return
}

func (conn *TLSConnection) SetReadTimeout(d time.Duration) (err error) {
	conn.fd.SetReadTimeout(d)
	return
}

func (conn *TLSConnection) SetWriteTimeout(d time.Duration) (err error) {
	conn.fd.SetWriteTimeout(d)
	return
}

func (conn *TLSConnection) SetReadBuffer(n int) (err error) {
	if err = aio.SetReadBuffer(conn.fd, n); err != nil {
		err = aio.NewOpErr(aio.OpSet, conn.fd, err)
		return
	}
	return
}

func (conn *TLSConnection) SetWriteBuffer(n int) (err error) {
	if err = aio.SetWriteBuffer(conn.fd, n); err != nil {
		err = aio.NewOpErr(aio.OpSet, conn.fd, err)
		return
	}
	return
}

const (
	defaultReadBufferSize = 1024
)

func (conn *TLSConnection) SetInboundBuffer(n int) {
	if n < 1 {
		n = defaultReadBufferSize
	}
	conn.rbs = n
	return
}

func (conn *TLSConnection) MultipathTCP() bool {
	return aio.IsUsingMultipathTCP(conn.fd)
}

func (conn *TLSConnection) SetNoDelay(noDelay bool) (err error) {
	err = aio.SetNoDelay(conn.fd, noDelay)
	return
}

func (conn *TLSConnection) SetLinger(sec int) (err error) {
	err = aio.SetLinger(conn.fd, sec)
	return
}

func (conn *TLSConnection) SetKeepAlive(keepalive bool) (err error) {
	err = aio.SetKeepAlive(conn.fd, keepalive)
	return
}

func (conn *TLSConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
	err = aio.SetKeepAlivePeriod(conn.fd, period)
	return
}

func (conn *TLSConnection) SetKeepAliveConfig(config aio.KeepAliveConfig) (err error) {
	err = aio.SetKeepAliveConfig(conn.fd, config)
	return
}

func (conn *TLSConnection) Read() (future async.Future[transport.Inbound]) {

	return
}

func (conn *TLSConnection) Write(b []byte) (future async.Future[transport.Outbound]) {

	return
}

func (conn *TLSConnection) Sendfile(file string) (future async.Future[transport.Outbound]) {

	return
}

func (conn *TLSConnection) Close() (future async.Future[async.Void]) {
	promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode())
	if promiseErr != nil {
		conn.rb.Close()
		aio.CloseImmediately(conn.fd)
		timeslimiter.TryRevert(conn.ctx)
		future = async.FailedImmediately[async.Void](conn.ctx, aio.NewOpErr(aio.OpClose, conn.fd, promiseErr))
		return
	}
	aio.Close(conn.fd, func(result int, userdata aio.Userdata, err error) {
		if err != nil {
			promise.Fail(aio.NewOpErr(aio.OpClose, conn.fd, err))
		} else {
			promise.Succeed(async.Void{})
		}
		conn.rb.Close()
		timeslimiter.TryRevert(conn.ctx)
		return
	})
	future = promise.Future()
	return
}

// sessionState returns a partially filled-out [SessionState] with information
// from the current connection.
func (conn *TLSConnection) sessionState() *SessionState {
	return &SessionState{
		version:           conn.vers,
		cipherSuite:       conn.cipherSuite,
		createdAt:         uint64(conn.config.time().Unix()),
		alpnProtocol:      conn.clientProtocol,
		peerCertificates:  conn.peerCertificates,
		activeCertHandles: conn.activeCertHandles,
		ocspResponse:      conn.ocspResponse,
		scts:              conn.scts,
		isClient:          conn.isClient,
		extMasterSecret:   conn.extMasterSecret,
		verifiedChains:    conn.verifiedChains,
	}
}
