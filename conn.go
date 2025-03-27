package rio

import (
	"net"
)

type Conn interface {
	net.Conn
	// SetSendZC set to use prep_sendzc
	SetSendZC(ok bool) bool
}
