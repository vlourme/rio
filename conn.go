package rio

import (
	"net"
)

type Conn interface {
	net.Conn
	// EnableSendZC try to enable send_zc
	EnableSendZC(enable bool) bool
}
