package security

import (
	"bytes"
	"context"
	"crypto/cipher"
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
	Write(b []byte) (future async.Future[int])
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

func (conn *TLSConnection) Write(b []byte) (future async.Future[int]) {

	return
}

func (conn *TLSConnection) Sendfile(file string) (future async.Future[int]) {

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
		future = conn.closeSocket()
		return
	}

	if conn.isHandshakeComplete.Load() {
		promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode())
		if promiseErr != nil {
			future = conn.closeSocket()
			return
		}
		future = promise.Future()
		conn.closeNotify().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
			var alertErr error
			if cause != nil {
				alertErr = fmt.Errorf("tls: failed to send closeNotify alert (but connection was closed anyway): %w", cause)
			}
			conn.closeSocket().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
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
		future = conn.closeSocket()
	}
	return
}

func (conn *TLSConnection) closeSocket() (future async.Future[async.Void]) {
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

	if errors.Is(err, alertNoRenegotiation) || errors.Is(err, alertCloseNotify) {
		conn.tmp[0] = alertLevelWarning
	} else {
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

func (conn *TLSConnection) write(b []byte) (future async.Future[int]) {
	if conn.buffering {
		conn.sendBuf = append(conn.sendBuf, b...)
		future = async.SucceedImmediately[int](conn.ctx, len(b))
		return
	}
	promise, promiseErr := async.Make[int](conn.ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[int](conn.ctx, promiseErr)
		return
	}
	future = promise.Future()

	aio.Send(conn.fd, b, func(result int, userdata aio.Userdata, err error) {
		conn.bytesSent += int64(result)
		promise.Complete(result, err)
		return
	})

	return
}

func (conn *TLSConnection) flush() (future async.Future[int]) {
	if len(conn.sendBuf) == 0 {
		future = async.SucceedImmediately[int](conn.ctx, 0)
		return
	}
	promise, promiseErr := async.Make[int](conn.ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[int](conn.ctx, promiseErr)
		return
	}
	future = promise.Future()

	aio.Send(conn.fd, conn.sendBuf, func(result int, userdata aio.Userdata, err error) {
		conn.bytesSent += int64(result)
		conn.sendBuf = nil
		conn.buffering = false

		promise.Complete(result, err)
		return
	})

	return
}

const (
	// tcpMSSEstimate is a conservative estimate of the TCP maximum segment
	// size (MSS). A constant is used, rather than querying the kernel for
	// the actual MSS, to avoid complexity. The value here is the IPv6
	// minimum MTU (1280 bytes) minus the overhead of an IPv6 header (40
	// bytes) and a TCP header with timestamps (32 bytes).
	tcpMSSEstimate = 1208

	// recordSizeBoostThreshold is the number of bytes of application data
	// sent after which the TLS record size will be increased to the
	// maximum.
	recordSizeBoostThreshold = 128 * 1024
)

// maxPayloadSizeForWrite returns the maximum TLS payload size to use for the
// next application data record. There is the following trade-off:
//
//   - For latency-sensitive applications, such as web browsing, each TLS
//     record should fit in one TCP segment.
//   - For throughput-sensitive applications, such as large file transfers,
//     larger TLS records better amortize framing and encryption overheads.
//
// A simple heuristic that works well in practice is to use small records for
// the first 1MB of data, then use larger records for subsequent data, and
// reset back to smaller records after the connection becomes idle. See "High
// Performance Web Networking", Chapter 4, or:
// https://www.igvita.com/2013/10/24/optimizing-tls-record-size-and-buffering-latency/
//
// In the interests of simplicity and determinism, this code does not attempt
// to reset the record size once the connection is idle, however.
func (conn *TLSConnection) maxPayloadSizeForWrite(typ recordType) int {
	if conn.config.DynamicRecordSizingDisabled || typ != recordTypeApplicationData {
		return maxPlaintext
	}

	if conn.bytesSent >= recordSizeBoostThreshold {
		return maxPlaintext
	}

	// Subtract TLS overheads to get the maximum payload size.
	payloadBytes := tcpMSSEstimate - recordHeaderLen - conn.out.explicitNonceLen()
	if conn.out.cipher != nil {
		switch ciph := conn.out.cipher.(type) {
		case cipher.Stream:
			payloadBytes -= conn.out.mac.Size()
		case cipher.AEAD:
			payloadBytes -= ciph.Overhead()
		case cbcMode:
			blockSize := ciph.BlockSize()
			// The payload must fit in a multiple of blockSize, with
			// room for at least one padding byte.
			payloadBytes = (payloadBytes & ^(blockSize - 1)) - 1
			// The MAC is appended before padding so affects the
			// payload size directly.
			payloadBytes -= conn.out.mac.Size()
		default:
			panic("unknown cipher type")
		}
	}
	if conn.vers == VersionTLS13 {
		payloadBytes-- // encrypted ContentType
	}

	// Allow packet growth in arithmetic progression up to max.
	pkt := conn.packetsSent
	conn.packetsSent++
	if pkt > 1000 {
		return maxPlaintext // avoid overflow in multiply below
	}

	n := payloadBytes * int(pkt+1)
	if n > maxPlaintext {
		n = maxPlaintext
	}
	return n
}

// outBufPool pools the record-sized scratch buffers used by writeRecordLocked.
var outBufPool = sync.Pool{
	New: func() any {
		return new([]byte)
	},
}

// writeRecordLocked writes a TLS record with the given type and payload to the
// connection and updates the record layer state.
func (conn *TLSConnection) writeRecordLocked(typ recordType, data []byte) (future async.Future[int]) {
	promise, promiseErr := async.Make[int](conn.ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[int](conn.ctx, promiseErr)
		return
	}
	future = promise.Future()

	outBufPtr := outBufPool.Get().(*[]byte)
	outBuf := *outBufPtr

	conn.writeRecordLocked0(typ, data, 0, outBuf).OnComplete(func(ctx context.Context, entry int, cause error) {
		*outBufPtr = outBuf
		outBufPool.Put(outBufPtr)

		promise.Complete(entry, cause)
		return
	})

	return
}

func (conn *TLSConnection) writeRecordLocked0(typ recordType, data []byte, written int, outBuf []byte) (future async.Future[int]) {
	promise, promiseErr := async.Make[int](conn.ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[int](conn.ctx, promiseErr)
		return
	}
	future = promise.Future()

	if len(data) == 0 {
		if typ == recordTypeChangeCipherSpec && conn.vers != VersionTLS13 {
			if err := conn.out.changeCipherSpec(); err != nil {
				conn.sendAlertLocked(err.(alert)).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					promise.Complete(written, cause)
					return
				})
				return
			}
		}
		promise.Succeed(written)
		return
	}

	m := len(data)
	if maxPayload := conn.maxPayloadSizeForWrite(typ); m > maxPayload {
		m = maxPayload
	}

	_, outBuf = sliceForAppend(outBuf[:0], recordHeaderLen)
	outBuf[0] = byte(typ)
	vers := conn.vers
	if vers == 0 {
		// Some TLS servers fail if the record version is
		// greater than TLS 1.0 for the initial ClientHello.
		vers = VersionTLS10
	} else if vers == VersionTLS13 {
		// TLS 1.3 froze the record layer version to 1.2.
		// See RFC 8446, Section 5.1.
		vers = VersionTLS12
	}
	outBuf[1] = byte(vers >> 8)
	outBuf[2] = byte(vers)
	outBuf[3] = byte(m >> 8)
	outBuf[4] = byte(m)

	var encryptErr error
	outBuf, encryptErr = conn.out.encrypt(outBuf, data[:m], conn.config.rand())
	if encryptErr != nil {
		promise.Complete(written, encryptErr)
		return
	}

	conn.write(outBuf).OnComplete(func(ctx context.Context, entry int, cause error) {
		written += entry

		if cause != nil {
			promise.Complete(written, cause)
			return
		}

		data = data[m:]

		if len(data) == 0 {
			if typ == recordTypeChangeCipherSpec && conn.vers != VersionTLS13 {
				if err := conn.out.changeCipherSpec(); err != nil {
					conn.sendAlertLocked(err.(alert)).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
						promise.Complete(written, cause)
						return
					})
					return
				}
			}
			promise.Succeed(written)
			return
		}

		conn.writeRecordLocked0(typ, data, written, outBuf).OnComplete(func(ctx context.Context, entry int, cause error) {
			written += entry
			promise.Complete(written, cause)
			return
		})
		return
	})
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
