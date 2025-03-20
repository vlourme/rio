//go:build linux

package cbpf

import (
	"fmt"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
	"unsafe"
)

const (
	skfAdOffPlusKSkfAdCPU = 4294963236
	cpuIdSize             = 4
)

func NewFilter(cpus uint32) Filter {
	return Filter{
		bpf.LoadAbsolute{Off: skfAdOffPlusKSkfAdCPU, Size: cpuIdSize},
		bpf.ALUOpConstant{Op: bpf.ALUOpMod, Val: cpus},
		bpf.RetA{},
	}
}

type Filter []bpf.Instruction

func (f Filter) ApplyTo(fd int) error {
	var (
		err       error
		assembled []bpf.RawInstruction
	)

	if assembled, err = bpf.Assemble(f); err != nil {
		return fmt.Errorf("BPF filter assemble error: %w", err)
	}
	program := unix.SockFprog{
		Len:    uint16(len(assembled)),
		Filter: (*unix.SockFilter)(unsafe.Pointer(&assembled[0])),
	}
	b := (*[unix.SizeofSockFprog]byte)(unsafe.Pointer(&program))[:unix.SizeofSockFprog]

	if _, _, errno := syscall.Syscall6(
		syscall.SYS_SETSOCKOPT,
		uintptr(fd), uintptr(syscall.SOL_SOCKET), uintptr(unix.SO_ATTACH_REUSEPORT_CBPF),
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0,
	); errno != 0 {
		return os.NewSyscallError("sys_setsocketopt", errno)
	}
	return nil
}
