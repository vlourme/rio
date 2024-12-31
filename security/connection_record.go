package security

import (
	"crypto/tls"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
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

func (conn *connection) writeRecordLocked(typ recordType, data []byte) (future async.Future[int]) {

	return
}

func (conn *connection) newRecordHeaderError(c *connection, raw []byte, msg string) (err tls.RecordHeaderError) {
	err.Msg = msg
	if c != nil {
		err.Conn = transport.AdaptToNetConn(c)
	}
	copy(err.RecordHeader[:], raw)
	return err
}
