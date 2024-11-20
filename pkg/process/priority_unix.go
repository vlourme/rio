//go:build unix

package process

import (
	"golang.org/x/sys/unix"
	"os"
)

func SetCurrentProcessPriority(level PriorityLeven) (err error) {
	pid := os.Getpid()
	n := 0
	switch level {
	case REALTIME:
		n = -19
		break
	case HIGH:
		n = -15
		break
	case NORM:
		n = 0
		break
	case IDLE:
		n = 15
		break
	}
	err = unix.Setpriority(unix.PRIO_PROCESS, pid, n)
	return
}
