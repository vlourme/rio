package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"net"
	"strings"
	"syscall"
	"time"
)

var (
	DefaultDialer = Dialer{
		Timeout:            15 * time.Second,
		Deadline:           time.Time{},
		LocalAddr:          nil,
		KeepAlive:          0,
		KeepAliveConfig:    net.KeepAliveConfig{Enable: true},
		MultipathTCP:       false,
		FastOpen:           false,
		QuickAck:           false,
		SendZC:             false,
		AutoFixedFdInstall: false,
		Control:            nil,
		ControlContext:     nil,
	}
)

// Dial connects to the address on the named network.
//
// Known networks are "tcp", "tcp4" (IPv4-only), "tcp6" (IPv6-only),
// "udp", "udp4" (IPv4-only), "udp6" (IPv6-only), "ip", "ip4"
// (IPv4-only), "ip6" (IPv6-only), "unix", "unixgram" and
// "unixpacket".
//
// For TCP and UDP networks, the address has the form "host:port".
// The host must be a literal IP address, or a host name that can be
// resolved to IP addresses.
// The port must be a literal port number or a service name.
// If the host is a literal IPv6 address it must be enclosed in square
// brackets, as in "[2001:db8::1]:80" or "[fe80::1%zone]:80".
// The zone specifies the scope of the literal IPv6 address as defined
// in RFC 4007.
// The functions [JoinHostPort] and [SplitHostPort] manipulate a pair of
// host and port in this form.
// When using TCP, and the host resolves to multiple IP addresses,
// Dial will try each IP address in order until one succeeds.
//
// Examples:
//
//	Dial("tcp", "golang.org:http")
//	Dial("tcp", "192.0.2.1:http")
//	Dial("tcp", "198.51.100.1:80")
//	Dial("udp", "[2001:db8::1]:domain")
//	Dial("udp", "[fe80::1%lo0]:53")
//	Dial("tcp", ":80")
//
// For IP networks, the network must be "ip", "ip4" or "ip6" followed
// by a colon and a literal protocol number or a protocol name, and
// the address has the form "host". The host must be a literal IP
// address or a literal IPv6 address with zone.
// It depends on each operating system how the operating system
// behaves with a non-well known protocol number such as "0" or "255".
//
// Examples:
//
//	Dial("ip4:1", "192.0.2.1")
//	Dial("ip6:ipv6-icmp", "2001:db8::1")
//	Dial("ip6:58", "fe80::1%lo0")
//
// For TCP, UDP and IP networks, if the host is empty or a literal
// unspecified IP address, as in ":80", "0.0.0.0:80" or "[::]:80" for
// TCP and UDP, "", "0.0.0.0" or "::" for IP, the local system is
// assumed.
//
// For Unix networks, the address must be a file system path.
func Dial(network string, address string) (net.Conn, error) {
	ctx := context.Background()
	return DialContext(ctx, network, address)
}

// DialContext same as [Dial] but takes a context.
func DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	dialer := DefaultDialer
	if strings.HasPrefix(network, "tcp") {
		dialer.SetFastOpen(true)
		dialer.SetQuickAck(true)
	}
	return dialer.DialContext(ctx, network, address)
}

// DialTimeout acts like [Dial] but takes a timeout.
//
// The timeout includes name resolution, if required.
// When using TCP, and the host in the address parameter resolves to
// multiple IP addresses, the timeout is spread over each consecutive
// dial, such that each is given an appropriate fraction of the time
// to connect.
//
// See func Dial for a description of the network and address
// parameters.
func DialTimeout(network string, address string, timeout time.Duration) (net.Conn, error) {
	ctx := context.Background()
	return DialContextTimeout(ctx, network, address, timeout)
}

// DialContextTimeout acts like [DialTimeout] but takes a context.
func DialContextTimeout(ctx context.Context, network string, address string, timeout time.Duration) (net.Conn, error) {
	dialer := DefaultDialer
	dialer.Timeout = timeout
	if strings.HasPrefix(network, "tcp") {
		dialer.SetFastOpen(true)
		dialer.SetQuickAck(true)
	}
	return dialer.DialContext(ctx, network, address)
}

type Dialer struct {
	net.Dialer
	// Timeout is the maximum amount of time a dial will wait for
	// a connect to complete. If Deadline is also set, it may fail
	// earlier.
	//
	// The default is no timeout.
	//
	// When using TCP and dialing a host name with multiple IP
	// addresses, the timeout may be divided between them.
	//
	// With or without a timeout, the operating system may impose
	// its own earlier timeout. For instance, TCP timeouts are
	// often around 3 minutes.
	Timeout time.Duration
	// Deadline is the absolute point in time after which dials
	// will fail. If Timeout is set, it may fail earlier.
	// Zero means no deadline, or dependent on the operating system
	// as with the Timeout option.
	Deadline time.Time
	// KeepAlive specifies the interval between keep-alive
	// probes for an active network connection.
	//
	// KeepAlive is ignored if KeepAliveConfig.Enable is true.
	//
	// If zero, keep-alive probes are sent with a default value
	// (currently 15 seconds), if supported by the protocol and operating
	// system. Network protocols or operating systems that do
	// not support keep-alive ignore this field.
	// If negative, keep-alive probes are disabled.
	KeepAlive time.Duration
	// KeepAliveConfig specifies the keep-alive probe configuration
	// for an active network connection, when supported by the
	// protocol and operating system.
	//
	// If KeepAliveConfig.Enable is true, keep-alive probes are enabled.
	// If KeepAliveConfig.Enable is false and KeepAlive is negative,
	// keep-alive probes are disabled.
	KeepAliveConfig net.KeepAliveConfig
	// LocalAddr is the local address to use when dialing an
	// address. The address must be of a compatible type for the
	// network being dialed.
	// If nil, a local address is automatically chosen.
	LocalAddr net.Addr
	// FallbackDelay specifies the length of time to wait before
	// spawning a RFC 6555 Fast Fallback connection. That is, this
	// is the amount of time to wait for IPv6 to succeed before
	// assuming that IPv6 is misconfigured and falling back to
	// IPv4.
	//
	// If zero, a default delay of 300ms is used.
	// A negative value disables Fast Fallback support.
	FallbackDelay time.Duration
	// If MultipathTCP is set to a value allowing Multipath TCP (MPTCP) to be
	// used, any call to Dial with "tcp(4|6)" as network will use MPTCP if
	// supported by the operating system.
	MultipathTCP bool
	// FastOpen is set TCP_FASTOPEN
	FastOpen bool
	// QuickAck is set TCP_QUICKACK
	QuickAck bool
	// SendZC is set IOURING.OP_SENDZC
	SendZC bool
	// AutoFixedFdInstall is set install conn fd into iouring after accepted.
	AutoFixedFdInstall bool
	// If Control is not nil, it is called after creating the network
	// connection but before actually dialing.
	//
	// Network and address parameters passed to Control function are not
	// necessarily the ones passed to Dial. Calling Dial with TCP networks
	// will cause the Control function to be called with "tcp4" or "tcp6",
	// UDP networks become "udp4" or "udp6", IP networks become "ip4" or "ip6",
	// and other known networks are passed as-is.
	//
	// Control is ignored if ControlContext is not nil.
	Control func(network, address string, c syscall.RawConn) error
	// If ControlContext is not nil, it is called after creating the network
	// connection but before actually dialing.
	//
	// Network and address parameters passed to ControlContext function are not
	// necessarily the ones passed to Dial. Calling Dial with TCP networks
	// will cause the ControlContext function to be called with "tcp4" or "tcp6",
	// UDP networks become "udp4" or "udp6", IP networks become "ip4" or "ip6",
	// and other known networks are passed as-is.
	//
	// If ControlContext is not nil, Control is ignored.
	ControlContext func(ctx context.Context, network, address string, c syscall.RawConn) error
	// Vortex customize [aio.Vortex]
	Vortex *aio.Vortex
}

// SetFastOpen set fast open.
func (d *Dialer) SetFastOpen(use bool) {
	d.FastOpen = use
}

// SetQuickAck set quick ack.
func (d *Dialer) SetQuickAck(use bool) {
	d.QuickAck = use
}

// SetMultipathTCP set multi-path tcp.
func (d *Dialer) SetMultipathTCP(use bool) {
	d.MultipathTCP = use
}

// SetSendZC set send zero-copy.
//
// available after 6.0
func (d *Dialer) SetSendZC(use bool) {
	d.SendZC = use
}

// SetAutoFixedFdInstall set auto install fixed fd.
//
// auto install fixed fd when connected.
// available after [RIO_IOURING_REG_FIXED_FILES] set.
func (d *Dialer) SetAutoFixedFdInstall(auto bool) {
	d.AutoFixedFdInstall = auto
}

// SetVortex set customize [aio.Vortex].
func (d *Dialer) SetVortex(v *aio.Vortex) {
	d.Vortex = v
}

func (d *Dialer) deadline(ctx context.Context, now time.Time) (earliest time.Time) {
	if d.Timeout != 0 {
		earliest = now.Add(d.Timeout)
	}
	if deadline, ok := ctx.Deadline(); ok {
		earliest = minNonzeroTime(earliest, deadline)
	}
	return minNonzeroTime(earliest, d.Deadline)
}

func (d *Dialer) dualStack() bool { return d.FallbackDelay >= 0 }

func (d *Dialer) fallbackDelay() time.Duration {
	if d.FallbackDelay > 0 {
		return d.FallbackDelay
	} else {
		return 300 * time.Millisecond
	}
}

func minNonzeroTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.Before(b) {
		return a
	}
	return b
}
