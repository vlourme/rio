package tls

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio"
	"strings"
)

func DialWithDialer(dialer *rio.Dialer, network, addr string, config *tls.Config) (*tls.Conn, error) {
	return dial(context.Background(), dialer, network, addr, config)
}

func Dial(network, addr string, config *tls.Config) (*tls.Conn, error) {
	return DialWithDialer(&rio.DefaultDialer, network, addr, config)
}

func dial(ctx context.Context, netDialer *rio.Dialer, network, addr string, config *tls.Config) (*tls.Conn, error) {
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
		config = &tls.Config{}
	}
	// If no ServerName is set, infer the ServerName
	// from the hostname we're connecting to.
	if config.ServerName == "" {
		// Make a copy to avoid polluting argument or default.
		c := config.Clone()
		c.ServerName = hostname
		config = c
	}

	conn := tls.Client(rawConn, config)
	if err := conn.HandshakeContext(ctx); err != nil {
		_ = rawConn.Close()
		return nil, err
	}
	return conn, nil
}
