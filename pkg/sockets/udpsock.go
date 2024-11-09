package sockets

type ListenUPDHandler func(conn UDPConnection, err error)

func ListenUPD(handler ListenUPDHandler) {
	// udp addr
	// conn socket
	// conn
}
