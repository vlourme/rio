package rio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"net"
)

type FixedReaderWriter interface {
	AcquireRegisteredBuffer() *aio.FixedBuffer
	ReleaseRegisteredBuffer(buf *aio.FixedBuffer)
	ReadFixed(buf *aio.FixedBuffer) (n int, err error)
	WriteFixed(buf *aio.FixedBuffer) (n int, err error)
}

// Fixed
// 转为固定读写。
// 必须设定 IOURING_REG_BUFFERS (大小, 个数)。
// 如 setenv IOURING_REG_BUFFERS 1024, 1000
func Fixed(c net.Conn) (fixed FixedReaderWriter, ok bool) {
	fixed, ok = c.(FixedReaderWriter)
	return
}

type FixedFd interface {
	InstallFixedFd() (err error)
	FixedFdInstalled() bool
}

func FixedFdInstaller(v any) (fixed FixedFd, ok bool) {
	fixed, ok = v.(FixedFd)
	return
}
