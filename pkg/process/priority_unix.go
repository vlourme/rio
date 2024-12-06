//go:build unix

package process

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func SetCurrentProcessPriority(level PriorityLevel) (err error) {
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
	default:
		return errors.New("invalid priority level")
	}
	err = unix.Setpriority(unix.PRIO_PROCESS, pid, n)
	return
}
