package tls

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio"
	"net"
	"strings"
)

type Dialer struct {
	NetDialer *rio.Dialer
	Config    *Config
}

func (d *Dialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

func (d *Dialer) netDialer() *rio.Dialer {
	if d.NetDialer != nil {
		return d.NetDialer
	}
	return &rio.DefaultDialer
}

func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c, err := dial(ctx, d.netDialer(), network, addr, d.Config)
	if err != nil {
		// Don't return c (a typed nil) in an interface.
		return nil, err
	}
	return c, nil
}

var emptyConfig Config

func defaultConfig() *Config {
	return &emptyConfig
}

func DialWithDialer(dialer *rio.Dialer, network, addr string, config *Config) (*Conn, error) {
	return dial(context.Background(), dialer, network, addr, config)
}

func Dial(network, addr string, config *Config) (*Conn, error) {
	return DialWithDialer(&rio.DefaultDialer, network, addr, config)
}

func dial(ctx context.Context, netDialer *rio.Dialer, network, addr string, config *Config) (*Conn, error) {
	if netDialer.Timeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, netDialer.Timeout)
		defer cancel()
	}

	if !netDialer.Deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, netDialer.Deadline)
		defer cancel()
	}

	rawConn, err := netDialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	hostname := addr[:colonPos]

	if config == nil {
		config = defaultConfig()
	}
	if config.ServerName == "" {
		c := config.Clone()
		c.ServerName = hostname
		config = ConfigFrom(c)
	}

	conn := tls.Client(rawConn, ConfigTo(config))
	if hsErr := conn.HandshakeContext(ctx); hsErr != nil {
		_ = rawConn.Close()
		return nil, hsErr
	}
	return ConnFrom(conn), nil
}
