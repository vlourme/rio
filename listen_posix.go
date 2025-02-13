//go:build !linux

package rio

import "net"

// Listen
// 监听流
func Listen(network string, addr string, options ...Option) (ln net.Listener, err error) {
	return
}
