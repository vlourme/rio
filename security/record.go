package security

import (
	"context"
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

type RecordType uint8

const (
	RecordTypeChangeCipherSpec RecordType = 20
	RecordTypeAlert            RecordType = 21
	RecordTypeHandshake        RecordType = 22
	RecordTypeApplicationData  RecordType = 23
)

// TLS compression types.
const (
	CompressionNone uint8 = 0
)

func WriteRecord(ctx context.Context, writer transport.Writer, cipher CipherSuite, typ RecordType, data []byte) (future async.Future[async.Void]) {

	return
}

// RecordHeaderError is returned when a TLS record header is invalid.
type RecordHeaderError struct {
	// Msg contains a human readable string that describes the error.
	Msg string
	// RecordHeader contains the five bytes of TLS record header that
	// triggered the error.
	RecordHeader [5]byte
	// Transport provides the underlying transport.Transport in the case that a client
	// sent an initial handshake that didn't look like TLS.
	// It is nil if there's already been a handshake or a TLS Alert has
	// been written to the transport.
	Transport transport.Transport
}

func (e RecordHeaderError) Error() string { return "security: " + e.Msg }
