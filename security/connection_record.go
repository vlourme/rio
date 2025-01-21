package security

import (
	"context"
	"crypto/cipher"
	"crypto/tls"
	"fmt"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"io"
	"net"
)

const (
	maxPlaintext               = 16384        // maximum plaintext payload length
	maxCiphertext              = 16384 + 2048 // maximum ciphertext payload length
	maxCiphertextTLS13         = 16384 + 256  // maximum ciphertext length in TLS 1.3
	recordHeaderLen            = 5            // record header length
	maxHandshake               = 65536        // maximum handshake we support (protocol max is 16 MB)
	maxHandshakeCertificateMsg = 262144       // maximum certificate message size (256 KiB)
	maxUselessRecords          = 16           // maximum number of consecutive non-advancing records
)

type recordType uint8

const (
	recordTypeChangeCipherSpec recordType = 20
	recordTypeAlert            recordType = 21
	recordTypeHandshake        recordType = 22
	recordTypeApplicationData  recordType = 23
)

// TLS compression types.
const (
	compressionNone uint8 = 0
)

func (conn *connection) readRecordOrCCS(expectChangeCipherSpec bool) (future async.Future[async.Void]) {
	// todo
	// readRecordOrCCS 和 readRecord 的关系反一下，readRecordOrCCS 负责读一个 record，readRecord(readRecordOrCCS(false))
	// 一律返回 data（解密后的 payload）?
	// readRecord() 来读一个 record
	// 处理 record
	// set input/hand
	// 貌似 handshake 的 data 是不会跨 record，然后 app data 又有 input 管理，所以是否 返回 []byte，不要 hand buf。
	return
}

// readRecord
// 只读取一个 record，不解密。
func (conn *connection) readRecord() (future async.Future[[]byte]) {
	ctx := conn.Context()
	promise, promiseErr := async.Make[[]byte](ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[[]byte](ctx, promiseErr)
		return
	}
	future = promise.Future()

	conn.Connection.Read().OnComplete(func(ctx context.Context, inbound transport.Inbound, cause error) {
		if cause != nil {
			if e, ok := cause.(net.Error); !ok || !e.Temporary() {
				_ = conn.in.setErrorLocked(cause)
			}
			promise.Fail(cause)
			return
		}
		reader := inbound.Reader()
		rn := reader.Length()
		if rn < recordHeaderLen {
			conn.readRecord().OnComplete(func(ctx context.Context, record []byte, cause error) {
				promise.Complete(record, cause)
			})
			return
		}
		header := reader.Peek(recordHeaderLen)
		typ := recordType(header[0])

		// No valid TLS record has a type of 0x80, however SSLv2 handshakes
		// start with a uint16 length where the MSB is set and the first record
		// is always < 256 bytes long. Therefore typ == 0x80 strongly suggests
		// an SSLv2 client.
		if !conn.handshakeComplete.Load() && typ == 0x80 {
			conn.sendAlert(alertProtocolVersion).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				raw, _ := reader.Next(rn)
				cause = conn.in.setErrorLocked(conn.newRecordHeaderError(nil, raw, "unsupported SSLv2 handshake received"))
				promise.Fail(cause)
				return
			})
			return
		}

		vers := uint16(header[1])<<8 | uint16(header[2])
		expectedVers := conn.vers
		if expectedVers == VersionTLS13 {
			// All TLS 1.3 records are expected to have 0x0303 (1.2) after
			// the initial hello (RFC 8446 Section 5.1).
			expectedVers = VersionTLS12
		}

		payloadLen := int(header[3])<<8 | int(header[4])

		if conn.haveVers && vers != expectedVers {
			conn.sendAlert(alertProtocolVersion).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				raw, _ := reader.Next(rn)
				msg := fmt.Sprintf("received record with version %x when expecting version %x", vers, expectedVers)
				cause = conn.in.setErrorLocked(conn.newRecordHeaderError(nil, raw, msg))
				promise.Fail(cause)
				return
			})
			return
		}
		if !conn.haveVers {
			// First message, be extra suspicious: this might not be a TLS
			// client. Bail out before reading a full 'body', if possible.
			// The current max version is 3.3 so if the version is >= 16.0,
			// it's probably not real.
			if (typ != recordTypeAlert && typ != recordTypeHandshake) || vers >= 0x1000 {
				raw, _ := reader.Next(rn)
				cause = conn.in.setErrorLocked(conn.newRecordHeaderError(conn.Connection, raw, "first record does not look like a TLS handshake"))
				promise.Fail(cause)
				return
			}
		}
		if conn.vers == VersionTLS13 && payloadLen > maxCiphertextTLS13 || payloadLen > maxCiphertext {
			conn.sendAlert(alertRecordOverflow).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				raw, _ := reader.Next(rn)
				msg := fmt.Sprintf("oversized record received with length %d", payloadLen)
				cause = conn.in.setErrorLocked(conn.newRecordHeaderError(nil, raw, msg))
				promise.Fail(cause)
				return
			})
			return
		}

		recordLen := recordHeaderLen + payloadLen

		if recordLen < rn { // read more
			conn.readRecord().OnComplete(func(ctx context.Context, record []byte, cause error) {
				promise.Complete(record, cause)
			})
			return
		}

		record := make([]byte, recordLen)
		n, rErr := reader.Read(record)
		if rErr != nil {
			promise.Fail(rErr)
			return
		}
		if n != recordLen {
			promise.Fail(io.ErrShortBuffer)
			return
		}

		// todo process record(typ+payload)
		promise.Succeed(record)
	})
	return
}

func (conn *connection) writeRecordLocked(typ recordType, data []byte) (future async.Future[int]) {
	m := len(data)
	if maxPayload := conn.maxPayloadSizeForWrite(typ); m > maxPayload {
		m = maxPayload
	}
	// todo
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
func (conn *connection) maxPayloadSizeForWrite(typ recordType) int {
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

func (conn *connection) newRecordHeaderError(c transport.Connection, raw []byte, msg string) (err tls.RecordHeaderError) {
	err.Msg = msg
	if c != nil {
		err.Conn = transport.AdaptToNetConn(c)
	}
	copy(err.RecordHeader[:], raw)
	return err
}
