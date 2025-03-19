package rio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"net"
)

// FixedReaderWriter
// use fixed buffer to read and write, which is registered in iouring.
type FixedReaderWriter interface {
	// AcquireRegisteredBuffer
	// acquire a buffer.
	// Note! must release buffer after used.
	AcquireRegisteredBuffer() *aio.FixedBuffer
	// ReleaseRegisteredBuffer
	// release a buffer.
	ReleaseRegisteredBuffer(buf *aio.FixedBuffer)
	// ReadFixed
	// read via registered fixed buffer.
	ReadFixed(buf *aio.FixedBuffer) (n int, err error)
	// WriteFixed
	// write via registered fixed buffer.
	WriteFixed(buf *aio.FixedBuffer) (n int, err error)
}

// ConvertToFixedReaderWriter
// convert net.Conn to FixedReaderWriter.
func ConvertToFixedReaderWriter(c net.Conn) (fixed FixedReaderWriter, ok bool) {
	fixed, ok = c.(FixedReaderWriter)
	return
}

// FixedConn
// use to register fixed file into iouring.
type FixedConn interface {
	// RegisterDirectFd register fd into iouring and get a direct fd
	RegisterDirectFd() error
	// DirectFd get direct fd
	DirectFd() int
}

// ConvertToFixedConn
// convert net.Conn to FixedConn.
func ConvertToFixedConn(c net.Conn) (fixed FixedConn, ok bool) {
	fixed, ok = c.(FixedConn)
	return
}
