//go:build dragonfly || linux || netbsd || openbsd

package sockets

import (
	"golang.org/x/sys/unix"
)

const readMsgFlags = unix.MSG_CMSG_CLOEXEC

func setReadMsgCloseOnExec(oob []byte) {}
