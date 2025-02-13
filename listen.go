package rio

import (
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/transport"
	"net"
	"time"
	"unsafe"
)

type ListenOptions struct {
	// KeepAlive specifies the keep-alive period for network
	// connections accepted by this listener.
	//
	// KeepAlive is ignored if KeepAliveConfig.Enable is true.
	//
	// If zero, keep-alive are enabled if supported by the protocol
	// and operating system. Network protocols or operating systems
	// that do not support keep-alive ignore this field.
	// If negative, keep-alive are disabled.
	KeepAlive time.Duration

	// KeepAliveConfig specifies the keep-alive probe configuration
	// for an active network connection, when supported by the
	// protocol and operating system.
	//
	// If KeepAliveConfig.Enable is true, keep-alive probes are enabled.
	// If KeepAliveConfig.Enable is false and KeepAlive is negative,
	// keep-alive probes are disabled.
	KeepAliveConfig net.KeepAliveConfig
	MultipathTCP    bool
	FastOpen        int
}

type ListenOption func(options *ListenOptions) (err error)

// *********************************************************************************************************************

type ListenPacketOptions struct {
	Options
	MulticastUDPInterface *net.Interface
}

// ListenPacket
// 监听包
func ListenPacket(network string, addr string, options ...Option) (conn transport.PacketConnection, err error) {
	opts := ListenPacketOptions{}
	for _, o := range options {
		err = o((*Options)(unsafe.Pointer(&opts)))
		if err != nil {
			err = errors.New(
				"listen packet failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(err),
			)
			return
		}
	}

	// inner
	fd, listenErr := aio.Listen(network, addr, aio.ListenerOptions{
		MultipathTCP:       false,
		MulticastInterface: opts.MulticastUDPInterface,
	})

	if listenErr != nil {
		err = errors.New(
			"listen packet failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(listenErr),
		)
		return
	}

	// ctx
	ctx := Background()
	// conn
	conn = newPacketConnection(ctx, fd)
	return
}
