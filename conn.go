package rio

import (
	"net"
)

// Conn is a generic stream-oriented network connection.
//
// Multiple goroutines may invoke methods on a Conn simultaneously.
type Conn interface {
	net.Conn
	// SendZCEnable check sendzc enabled
	SendZCEnable() bool
	// SetSocketOptInt set socket option
	SetSocketOptInt(level int, optName int, optValue int) (err error)
	// GetSocketOptInt get socket option
	GetSocketOptInt(level int, optName int) (optValue int, err error)
}
