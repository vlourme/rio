//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

func Recv(fd NetFd, b []byte, cb OperationCallback) {

	return
}

func completeRecv(result int, op *Operator, err error) {

	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {

	return
}

func completeRecvFrom(result int, op *Operator, err error) {

	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {

	return
}

func completeRecvMsg(result int, op *Operator, err error) {

	return
}
