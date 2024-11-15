//go:build aix || darwin || freebsd || solaris

package sockets

import (
	"golang.org/x/sys/unix"
)

const readMsgFlags = 0

func setReadMsgCloseOnExec(oob []byte) {
	scms, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}

	for _, scm := range scms {
		if scm.Header.Level == unix.SOL_SOCKET && scm.Header.Type == unix.SCM_RIGHTS {
			fds, err := unix.ParseUnixRights(&scm)
			if err != nil {
				continue
			}
			for _, fd := range fds {
				unix.CloseOnExec(fd)
			}
		}
	}
}
