//go:build netbsd

package aio

import (
	"errors"
	"golang.org/x/sys/unix"
	"runtime"
	"unsafe"
)

func (cylinder *KqueueCylinder) prepare(filter int16, flags uint16, op *Operator) (err error) {
	if cylinder.stopped.Load() {
		err = ErrUnexpectedCompletion
		return
	}
	ident := uint64(0)
	if op != nil {
		ident = uint64(op.fd.Fd())
	}
	entry := unix.Kevent_t{
		Ident:  ident,
		Filter: uint32(filter),
		Flags:  uint32(flags),
		Fflags: 0,
		Data:   0,
		Udata:  int64(uintptr(unsafe.Pointer(op))),
	}
	if ok := cylinder.submit(&entry); !ok {
		err = ErrBusy
		return
	}
	return
}

func (cylinder *KqueueCylinder) Loop(beg func(), end func()) {
	beg()
	defer end()

	kqfd := cylinder.fd
	changes := make([]unix.Kevent_t, cylinder.sq.capacity)
	events := make([]unix.Kevent_t, cylinder.sq.capacity)
	for {
		if cylinder.stopped.Load() {
			break
		}
		peeked := cylinder.sq.PeekBatch(changes)
		n, err := unix.Kevent(kqfd, changes[:peeked], events, nil)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			// todo handle err
			break
		}
		if n == 0 {
			continue
		}
		for i := 0; i < n; i++ {
			event := events[i]
			if event.Ident == 0 {
				if event.Udata == 0 {
					// stop
					cylinder.stopped.Store(true)
					break
				}
				// todo hande wakeup
				//op := (*Operator)(unsafe.Pointer(uintptr(event.Udata)))

				continue
			}
			cylinder.completing.Add(1)
			op := (*Operator)(unsafe.Pointer(uintptr(event.Udata)))
			if completion := op.completion; completion != nil {
				if event.Filter&unix.EVFILT_READ != 0 {
					// todo handle recv|recv_from|recv_msg in callback
					completion(0, op, nil)
				} else if event.Filter&unix.EVFILT_WRITE != 0 {
					// todo handle recv|recv_from|recv_msg in callback
					completion(0, op, nil)
				} else {
					completion(0, op, errors.New("aio.KqueueCylinder: unsupported filter"))
				}
				runtime.KeepAlive(op)
				op.callback = nil
				op.completion = nil
			}
			cylinder.completing.Add(-1)
		}
	}
	if kqfd > 0 {
		_ = unix.Close(kqfd)
	}
}
