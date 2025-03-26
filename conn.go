package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio"
	"net"
)

type Conn interface {
	net.Conn
	// SetAsync set sqe_flags |= IOSQE_ASYNC
	SetAsync(async bool) error
	// SetSendZC set to use prep_sendzc
	SetSendZC(ok bool) bool
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
	// RegisterDirectFd register fd into iouring and get a direct fd
	RegisterDirectFd() error
}
