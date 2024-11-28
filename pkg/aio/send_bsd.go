//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import "net"

func Send(fd NetFd, b []byte, cb OperationCallback) {

	return
}

func completeSend(result int, op *Operator, err error) {

	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {

	return
}

func completeSendTo(result int, op *Operator, err error) {

	return
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {

	return
}

func completeSendMsg(result int, op *Operator, err error) {

	return
}
