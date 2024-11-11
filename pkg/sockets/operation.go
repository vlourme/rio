package sockets

const (
	tcpAccept OperationMode = iota + 1
	tcpConnect
	unixAccept
	unixConnect
	read
	write
	readFrom
	writeTo
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
	case unixAccept:
		return "unixAccept"
	case unixConnect:
		return "unixConnect"
	case read:
		return "read"
	case write:
		return "write"
	case writeTo:
		return "writeTo"
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
