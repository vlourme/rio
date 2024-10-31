package sockets

const (
	accept OperationMode = iota + 1
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
	disconnect
	exit
)

type OperationMode int

func (op OperationMode) IsAccept() bool {
	return op == accept
}

func (op OperationMode) IsRead() bool {
	return op == read
}

func (op OperationMode) IsWrite() bool {
	return op == write
}

func (op OperationMode) String() string {
	switch op {
	case accept:
		return "accept"
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
	case disconnect:
		return "disconnect"
	case exit:
		return "exit"
	default:
		return "unknown"
	}
}
