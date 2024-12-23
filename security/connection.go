package security

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
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

var errEarlyCloseWrite = errors.New("tls: CloseWrite called before handshake complete")

func (conn *TLSConnection) CloseWrite() (future async.Future[async.Void]) {
	if !conn.isHandshakeComplete.Load() {
		future = async.FailedImmediately[async.Void](conn.ctx, errEarlyCloseWrite)
		return
	}
	future = conn.closeNotify()
	return
}

func (conn *TLSConnection) Close() (future async.Future[async.Void]) {
	// Interlock with Conn.Write above.
	var x int32
	for {
		x = conn.activeCall.Load()
		if x&1 != 0 {
			future = async.FailedImmediately[async.Void](conn.ctx, net.ErrClosed)
			return
		}
		if conn.activeCall.CompareAndSwap(x, x|1) {
			break
		}
	}
	if x != 0 {
		// io.Writer and io.Closer should not be used concurrently.
		// If Close is called while a Write is currently in-flight,
		// interpret that as a sign that this Close is really just
		// being used to break the Write and/or clean up resources and
		// avoid sending the alertCloseNotify, which may block
		// waiting on handshakeMutex or the c.out mutex.
		future = conn.closesocket()
		return
	}

	if conn.isHandshakeComplete.Load() {
		promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode())
		if promiseErr != nil {
			future = conn.closesocket()
			return
		}
		future = promise.Future()
		conn.closeNotify().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
			var alertErr error
			if cause != nil {
				alertErr = fmt.Errorf("tls: failed to send closeNotify alert (but connection was closed anyway): %w", cause)
			}
			conn.closesocket().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				if cause != nil {
					promise.Fail(cause)
					return
				}
				if alertErr != nil {
					promise.Fail(alertErr)
					return
				}
				promise.Succeed(async.Void{})
				return
			})
		})
	} else {
		future = conn.closesocket()
	}
	return
}

func (conn *TLSConnection) closesocket() (future async.Future[async.Void]) {
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

func (conn *TLSConnection) closeNotify() (future async.Future[async.Void]) {
	conn.out.Lock()
	promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode())
	if promiseErr != nil {
		conn.out.Unlock()
		future = async.FailedImmediately[async.Void](conn.ctx, promiseErr)
		return
	}
	future = promise.Future()

	_ = conn.SetWriteTimeout(5 * time.Second)
	conn.sendAlertLocked(alertCloseNotify).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		defer conn.out.Unlock()

		conn.closeNotifyErr = cause
		conn.closeNotifySent = true
		_ = conn.SetWriteTimeout(0)
		if cause != nil {
			promise.Fail(cause)
			return
		}
		promise.Succeed(async.Void{})
		return
	})

	return
}

func (conn *TLSConnection) sendAlertLocked(err alert) (future async.Future[async.Void]) {
	promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[async.Void](conn.ctx, promiseErr)
		return
	}
	future = promise.Future()

	switch err {
	case alertNoRenegotiation, alertCloseNotify:
		conn.tmp[0] = alertLevelWarning
	default:
		conn.tmp[0] = alertLevelError
	}
	conn.tmp[1] = byte(err)

	conn.writeRecordLocked(recordTypeAlert, conn.tmp[0:2]).OnComplete(func(ctx context.Context, entry int, cause error) {
		if errors.Is(cause, alertCloseNotify) {
			// closeNotify is a special case in that it isn't an error.
			promise.Fail(cause)
			return
		}
		cause = conn.out.setErrorLocked(&net.OpError{Op: "local error", Err: err})
		if cause != nil {
			promise.Fail(cause)
			return
		}
		promise.Succeed(async.Void{})
		return
	})
	return
}

func (conn *TLSConnection) sendAlert(err alert) (future async.Future[async.Void]) {
	conn.out.Lock()
	promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode())
	if promiseErr != nil {
		conn.out.Unlock()
		future = async.FailedImmediately[async.Void](conn.ctx, promiseErr)
		return
	}
	future = promise.Future()

	conn.sendAlertLocked(err).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		defer conn.out.Unlock()
		if cause != nil {
			promise.Fail(cause)
			return
		}
		promise.Succeed(async.Void{})
		return
	})

	return
}

func (conn *TLSConnection) writeRecordLocked(typ recordType, data []byte) (future async.Future[int]) {
	// todo
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
