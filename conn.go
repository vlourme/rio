package rio

import (
	"net"
)

type Conn interface {
	net.Conn
	// SendZCEnable check sendzc enabled
	SendZCEnable() bool
}
