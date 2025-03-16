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

// FixedFd
// it is used to install fd into iouring.
type FixedFd interface {
	// InstallFixedFd
	// install fixed fd into iouring.
	InstallFixedFd() (err error)
	// FixedFdInstalled
	// check installed.
	FixedFdInstalled() bool
}

// ConvertToFixedFd
// convert net.Conn, rio.TCPListener or rio.UnixListener to FixedFd.
func ConvertToFixedFd(v any) (fixed FixedFd, ok bool) {
	fixed, ok = v.(FixedFd)
	return
}
