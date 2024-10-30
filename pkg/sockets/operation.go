package sockets

const (
	Accept OperationMode = iota + 1
	Read
	Write
	// todo packet udp unix op
)

type OperationMode int

func (op OperationMode) IsAccept() bool {
	return op == Accept
}

func (op OperationMode) IsRead() bool {
	return op == Read
}

func (op OperationMode) IsWrite() bool {
	return op == Write
}

func (op OperationMode) String() string {
	switch op {
	case Accept:
		return "accept"
	case Read:
		return "read"
	case Write:
		return "write"
	default:
		return "unknown"
	}
}
