package sockets

const (
	tcpAccept OperationMode = iota + 1
	tcpConnect
	udpAccept
	unixAccept
	read
	write
	readFrom
	readFromUDP
	readFromUDPAddrPort
	readMsgUDP
	writeMsg
	readFromUnix
	readMsgUnix
)

type OperationMode int

func (op OperationMode) String() string {
	switch op {
	case tcpAccept:
		return "tcpAccept"
	case tcpConnect:
		return "tcpConnect"
	case udpAccept:
		return "udpAccept"
	case unixAccept:
		return "unixAccept"
	case read:
		return "read"
	case write:
		return "write"
	case writeMsg:
		return "writeMsg"
	case readFromUDPAddrPort:
		return "readFromUDPAddrPort"
	case readFrom:
		return "readFrom"
	case readFromUDP:
		return "readFromUDP"
	case readFromUnix:
		return "readFromUnix"
	case readMsgUDP:
		return "readMsgUDP"
	case readMsgUnix:
		return "readMsgUnix"
	default:
		return "unknown"
	}
}
