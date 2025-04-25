package rio

import (
	"context"
	"net"
	"syscall"
	"time"
)

// A Listener is a generic network listener for stream-oriented protocols.
//
// Multiple goroutines may invoke methods on a Listener simultaneously.
type Listener interface {
	net.Listener
	// SetSocketOptInt set socket option, the func is limited to SOL_SOCKET level.
	SetSocketOptInt(level int, optName int, optValue int) (err error)
	// GetSocketOptInt get socket option, the func is limited to SOL_SOCKET level.
	GetSocketOptInt(level int, optName int) (optValue int, err error)
}

// Listen announces on the local network address.
//
// The network must be "tcp", "tcp4", "tcp6", "unix" or "unixpacket".
//
// For TCP networks, if the host in the address parameter is empty or
// a literal unspecified IP address, Listen listens on all available
// unicast and anycast IP addresses of the local system.
// To only use IPv4, use network "tcp4".
// The address can use a host name, but this is not recommended,
// because it will create a listener for at most one of the host's IP
// addresses.
// If the port in the address parameter is empty or "0", as in
// "127.0.0.1:" or "[::1]:0", a port number is automatically chosen.
// The [Addr] method of [Listener] can be used to discover the chosen
// port.
//
// See func [Dial] for a description of the network and address
// parameters.
//
// Listen uses context.Background internally; to specify the context, use
// [ListenConfig.Listen].
func Listen(network string, addr string) (ln net.Listener, err error) {
	config := ListenConfig{}
	ctx := context.Background()
	ln, err = config.Listen(ctx, network, addr)
	return
}

// PacketConn is a generic packet-oriented network connection.
//
// Multiple goroutines may invoke methods on a PacketConn simultaneously.
type PacketConn interface {
	net.PacketConn
	// SetSocketOptInt set socket option, the func is limited to SOL_SOCKET level.
	SetSocketOptInt(level int, optName int, optValue int) (err error)
	// GetSocketOptInt get socket option, the func is limited to SOL_SOCKET level.
	GetSocketOptInt(level int, optName int) (optValue int, err error)
}

// ListenPacket announces on the local network address.
//
// The network must be "udp", "udp4", "udp6", "unixgram", or an IP
// transport. The IP transports are "ip", "ip4", or "ip6" followed by
// a colon and a literal protocol number or a protocol name, as in
// "ip:1" or "ip:icmp".
//
// For UDP and IP networks, if the host in the address parameter is
// empty or a literal unspecified IP address, ListenPacket listens on
// all available IP addresses of the local system except multicast IP
// addresses.
// To only use IPv4, use network "udp4" or "ip4:proto".
// The address can use a host name, but this is not recommended,
// because it will create a listener for at most one of the host's IP
// addresses.
// If the port in the address parameter is empty or "0", as in
// "127.0.0.1:" or "[::1]:0", a port number is automatically chosen.
// The LocalAddr method of [PacketConn] can be used to discover the
// chosen port.
//
// See func [Dial] for a description of the network and address
// parameters.
//
// ListenPacket uses context.Background internally; to specify the context, use
// [ListenConfig.ListenPacket].
func ListenPacket(network string, addr string) (c net.PacketConn, err error) {
	config := ListenConfig{}
	ctx := context.Background()
	c, err = config.ListenPacket(ctx, network, addr)
	return
}

// ListenConfig contains options for listening to an address.
type ListenConfig struct {
	// If Control is not nil, it is called after creating the network
	// connection but before binding it to the operating system.
	//
	// Network and address parameters passed to Control function are not
	// necessarily the ones passed to Listen. Calling Listen with TCP networks
	// will cause the Control function to be called with "tcp4" or "tcp6",
	// UDP networks become "udp4" or "udp6", IP networks become "ip4" or "ip6",
	// and other known networks are passed as-is.
	Control func(network, address string, c syscall.RawConn) error
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
	// MultipathTCP is set to a value allowing Multipath TCP (MPTCP) to be
	// used, any call to Listen with "tcp(4|6)" as network will use MPTCP if
	// supported by the operating system.
	MultipathTCP bool
	// ReusePort is set SO_REUSEPORT
	ReusePort bool
}

// SetMultipathTCP set multi-path tcp.
func (lc *ListenConfig) SetMultipathTCP(use bool) {
	lc.MultipathTCP = use
}
