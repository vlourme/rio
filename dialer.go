package rio

import (
	"context"
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
		UseSendZC:          false,
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
	Timeout            time.Duration
	Deadline           time.Time
	KeepAlive          time.Duration
	KeepAliveConfig    net.KeepAliveConfig
	LocalAddr          net.Addr
	MultipathTCP       bool
	FastOpen           bool
	QuickAck           bool
	UseSendZC          bool
	AutoFixedFdInstall bool
	Control            func(network, address string, c syscall.RawConn) error
	ControlContext     func(ctx context.Context, network, address string, c syscall.RawConn) error
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
	d.UseSendZC = use
}

// SetAutoFixedFdInstall set auto install fixed fd.
//
// auto install fixed fd when connected.
// available after [RIO_IOURING_REG_FIXED_FILES] set.
func (d *Dialer) SetAutoFixedFdInstall(auto bool) {
	d.AutoFixedFdInstall = auto
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

func minNonzeroTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.Before(b) {
		return a
	}
	return b
}
