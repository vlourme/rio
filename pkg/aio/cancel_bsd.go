//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"github.com/brickingsoft/errors"
	"syscall"
)

func CancelRead(fd Fd) {
	if op := fd.ROP(); op != nil {
		if received := op.received.Load(); received {
			return
		}
		fd.RemoveROP()
		if cb := op.callback; cb != nil {
			handle := fd.Fd()
			cylinder := fd.Cylinder().(*KqueueCylinder)
			if err := cylinder.prepareRW(handle, syscall.EVFILT_READ, syscall.EV_DELETE, op); err != nil {
				cb(Userdata{}, errors.From(ErrUnexpectedCompletion, errors.WithWrap(err)))
				return
			}
			cb(Userdata{}, errors.From(ErrUnexpectedCompletion))
			return
		}
	}
}

func CancelWrite(fd Fd) {
	if op := fd.WOP(); op != nil {
		if received := op.received.Load(); received {
			return
		}
		fd.RemoveWOP()
		if cb := op.callback; cb != nil {
			handle := fd.Fd()
			cylinder := fd.Cylinder().(*KqueueCylinder)
			if err := cylinder.prepareRW(handle, syscall.EVFILT_WRITE, syscall.EV_DELETE, op); err != nil {
				cb(Userdata{}, errors.From(ErrUnexpectedCompletion, errors.WithWrap(err)))
				return
			}
			cb(Userdata{}, errors.From(ErrUnexpectedCompletion))
			return
		}
	}
}
