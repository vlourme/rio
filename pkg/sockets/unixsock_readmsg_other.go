//go:build js || wasip1 || windows

package sockets

const readMsgFlags = 0

func setReadMsgCloseOnExec(oob []byte) {}
