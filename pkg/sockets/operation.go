package sockets

const (
	accept OperationMode = iota + 1
	dial
	read
	write
	readFrom
	writeTo
	readMsg
	writeMsg
)

type OperationMode int

func (op OperationMode) String() string {
	switch op {
	case accept:
		return "accept"
	case dial:
		return "dial"
	case read:
		return "read"
	case write:
		return "write"
	case readFrom:
		return "readFrom"
	case writeTo:
		return "writeTo"
	case readMsg:
		return "readMsg"
	case writeMsg:
		return "writeMsg"
	default:
		return "unknown"
	}
}
