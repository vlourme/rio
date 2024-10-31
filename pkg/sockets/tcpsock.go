package sockets

import (
	"context"
)

func ListenTCP(ctx context.Context, network, address string, opt Options) (ln Listener, err error) {
	//laddr, laddrErr := net.ResolveTCPAddr(network, address)
	//if laddrErr != nil {
	//	err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: err}
	//	return
	//}
	return
}

func DialTCP() (conn Connection, err error) {

	return
}
