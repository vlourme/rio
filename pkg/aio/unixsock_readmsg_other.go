//go:build windows

package aio

const readMsgFlags = 0

func setReadMsgCloseOnExec(oob []byte) {}
