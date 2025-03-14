//go:build linux

package sys

import (
	"os"
	"syscall"
	"unsafe"
)

func OpenEPoll() (*EPoll, error) {
	l := new(EPoll)
	p, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, os.NewSyscallError("epoll_create1", err)
	}
	l.fd = p
	r0, _, e0 := syscall.Syscall(syscall.SYS_EVENTFD2, 0, 0, 0)
	if e0 != 0 {
		_ = syscall.Close(p)
		return nil, os.NewSyscallError("evebtfd2", e0)
	}
	l.wfd = int(r0)
	l.AddRead(l.wfd)
	return l, nil
}

type EPoll struct {
	fd  int
	wfd int
}

func (p *EPoll) Wakeup() error {
	var x uint64 = 1
	_, err := syscall.Write(p.wfd, (*(*[8]byte)(unsafe.Pointer(&x)))[:])
	return err
}

func (p *EPoll) Wait(iter func(fd int, events uint32)) error {
	events := make([]syscall.EpollEvent, 64)
	for {
		n, err := syscall.EpollWait(p.fd, events, 100)
		if err != nil && err != syscall.EINTR {
			return err
		}
		for i := 0; i < n; i++ {
			if fd := int(events[i].Fd); fd != p.wfd {
				iter(fd, events[i].Events)
			} else if fd == p.wfd {
				var data [8]byte
				_, _ = syscall.Read(p.wfd, data[:])
			}
		}
	}
}

func (p *EPoll) Close() error {
	if err := syscall.Close(p.wfd); err != nil {
		return err
	}
	return syscall.Close(p.fd)
}

func (p *EPoll) AddReadWrite(fd int) {
	if err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_ADD, fd,
		&syscall.EpollEvent{Fd: int32(fd),
			Events: syscall.EPOLLIN | syscall.EPOLLOUT,
		},
	); err != nil {
		panic(err)
	}
}

func (p *EPoll) AddRead(fd int) {
	if err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_ADD, fd,
		&syscall.EpollEvent{Fd: int32(fd),
			Events: syscall.EPOLLIN,
		},
	); err != nil {
		panic(err)
	}
}

func (p *EPoll) ModRead(fd int) {
	if err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, fd,
		&syscall.EpollEvent{Fd: int32(fd),
			Events: syscall.EPOLLIN,
		},
	); err != nil {
		panic(err)
	}
}

func (p *EPoll) ModReadWrite(fd int) {
	if err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_MOD, fd,
		&syscall.EpollEvent{Fd: int32(fd),
			Events: syscall.EPOLLIN | syscall.EPOLLOUT,
		},
	); err != nil {
		panic(err)
	}
}

func (p *EPoll) ModDetach(fd int) {
	if err := syscall.EpollCtl(p.fd, syscall.EPOLL_CTL_DEL, fd,
		&syscall.EpollEvent{Fd: int32(fd),
			Events: syscall.EPOLLIN | syscall.EPOLLOUT,
		},
	); err != nil {
		panic(err)
	}
}
