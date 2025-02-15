package iouring

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	sysRegister = 427
)

const (
	RegisterBuffers uint32 = iota
	UnregisterBuffers
	RegisterFiles
	UnregisterFiles
	RegisterEventFd
	UnregisterEventFd
	RegisterFilesUpdate
	RegisterEventFDAsync
	RegisterProbe
	RegisterPersonality
	UnregisterPersonality
	RegisterRestrictions
	RegisterEnableRings
	RegisterFiles2
	RegisterFilesUpdate2
	RegisterBuffers2
	RegisterBuffersUpdate
	RegisterIOWQAff
	UnregisterIOWQAff
	RegisterIOWQMaxWorkers
	RegisterRingFDs
	UnregisterRingFDs
	RegisterPbufRing
	UnregisterPbufRing
	RegisterSyncCancel
	RegisterFileAllocRange
	RegisterLast
	RegisterUseRegisteredRing = 1 << 31
)

const (
	RestrictionRegisterOp uint32 = iota
	RestrictionSQEOp
	RestrictionSQEFlagsAllowed
	RestrictionSQEFlagsRequired
	RestrictionLast
)

const RsrcRegisterSparse uint32 = 1 << iota
const RegisterFilesSkip int = -2

type FilesUpdate struct {
	Offset uint32
	Resv   uint32
	Fds    uint64
}

type RsrcRegister struct {
	Nr    uint32
	Flags uint32
	Resv2 uint64
	Data  uint64
	Tags  uint64
}

type RsrcUpdate struct {
	Offset uint32
	Resv   uint32
	Data   uint64
}

type RsrcUpdate2 struct {
	Offset uint32
	Resv   uint32
	Data   uint64
	Tags   uint64
	Nr     uint32
	Resv2  uint32
}

type Restriction struct {
	OpCode  uint16
	OpFlags uint8
	Resv    uint8
	Resv2   [3]uint32
}

type SyncCancelReg struct {
	Addr    uint64
	Fd      int32
	Flags   uint32
	Timeout syscall.Timespec
	Pad     [4]uint64
}

type FileIndexRange struct {
	Off  uint32
	Len  uint32
	Resv uint64
}

func (ring *Ring) Register(fd int, opcode uint32, arg unsafe.Pointer, nrArgs uint32) (uint, syscall.Errno) {
	r1, _, errno := syscall.Syscall6(
		sysRegister,
		uintptr(fd),
		uintptr(opcode),
		uintptr(arg),
		uintptr(nrArgs),
		0,
		0,
	)
	return uint(r1), errno
}

func (ring *Ring) RegisterBuffersUpdateTag(off uint32, iovecs []syscall.Iovec, tags *uint64, nr uint32) (uint, error) {
	update := &RsrcUpdate2{
		Offset: off,
		Data:   uint64(uintptr(unsafe.Pointer(&iovecs[0]))),
		Tags:   *tags,
		Nr:     nr,
	}
	result, err := ring.doRegister(RegisterBuffersUpdate, unsafe.Pointer(update), uint32(unsafe.Sizeof(*update)))
	runtime.KeepAlive(update)
	return result, err
}

func (ring *Ring) RegisterBuffersTags(iovecs []syscall.Iovec, tags []uint64) (uint, error) {
	reg := &RsrcRegister{
		Nr:   uint32(len(tags)),
		Data: uint64(uintptr(unsafe.Pointer(&iovecs[0]))),
		Tags: uint64(uintptr(unsafe.Pointer(&tags[0]))),
	}
	result, err := ring.doRegister(RegisterBuffers2, unsafe.Pointer(reg), uint32(unsafe.Sizeof(*reg)))
	runtime.KeepAlive(reg)
	return result, err
}

func (ring *Ring) RegisterBuffersSparse(nr uint32) (uint, error) {
	reg := &RsrcRegister{
		Flags: RsrcRegisterSparse,
		Nr:    nr,
	}
	result, err := ring.doRegister(RegisterBuffers2, unsafe.Pointer(reg), uint32(unsafe.Sizeof(*reg)))
	runtime.KeepAlive(reg)
	return result, err
}

func (ring *Ring) RegisterBuffers(iovecs []syscall.Iovec) (uint, error) {
	return ring.doRegister(RegisterBuffers, unsafe.Pointer(&iovecs[0]), uint32(len(iovecs)))
}

func (ring *Ring) UnregisterBuffers() (uint, error) {
	return ring.doRegister(UnregisterBuffers, unsafe.Pointer(nil), 0)
}

func (ring *Ring) RegisterFilesUpdateTag(off uint, files []int, tags []uint64) (uint, error) {
	update := &RsrcUpdate2{
		Offset: uint32(off),
		Data:   uint64(uintptr(unsafe.Pointer(&files[0]))),
		Tags:   uint64(uintptr(unsafe.Pointer(&tags[0]))),
		Nr:     uint32(len(files)),
	}
	result, err := ring.doRegister(RegisterBuffers2, unsafe.Pointer(update), 1)
	runtime.KeepAlive(update)
	return result, err
}

func (ring *Ring) RegisterFilesUpdate(off uint, files []int) (uint, error) {
	update := &FilesUpdate{
		Offset: uint32(off),
		Fds:    uint64(uintptr(unsafe.Pointer(&files[0]))),
	}
	result, err := ring.doRegister(RegisterFilesUpdate, unsafe.Pointer(update), uint32(len(files)))
	runtime.KeepAlive(update)
	return result, err
}

func (ring *Ring) RegisterFilesSparse(nr uint32) (uint, error) {
	reg := &RsrcRegister{
		Flags: RsrcRegisterSparse,
		Nr:    nr,
	}
	var (
		ret         uint
		err         error
		errno       syscall.Errno
		didIncrease bool
	)
	for {
		ret, errno = ring.doRegisterErrno(RegisterFiles2, unsafe.Pointer(reg), uint32(unsafe.Sizeof(*reg)))
		if errno != 0 {
			break
		}
		if errno == syscall.EMFILE && !didIncrease {
			didIncrease = true
			err = increaseRlimitNoFile(uint64(nr))
			if err != nil {
				break
			}
			continue
		}
		break
	}
	return ret, err
}

func (ring *Ring) RegisterFilesTags(files []int, tags []uint64) (uint, error) {
	nr := len(files)
	reg := &RsrcRegister{
		Nr:   uint32(nr),
		Data: uint64(uintptr(unsafe.Pointer(&files[0]))),
		Tags: uint64(uintptr(unsafe.Pointer(&tags[0]))),
	}
	var (
		ret         uint
		err         error
		errno       syscall.Errno
		didIncrease bool
	)
	for {
		ret, errno = ring.doRegisterErrno(RegisterFiles2, unsafe.Pointer(reg), uint32(unsafe.Sizeof(*reg)))
		if ret > 0 {
			break
		}
		if errno == syscall.EMFILE && !didIncrease {
			didIncrease = true
			err = increaseRlimitNoFile(uint64(nr))
			if err != nil {
				break
			}
			continue
		}
		break
	}
	return ret, err
}

func (ring *Ring) RegisterFiles(files []int) (uint, error) {
	var (
		ret         uint
		err         error
		errno       syscall.Errno
		didIncrease bool
	)
	for {
		ret, errno = ring.doRegisterErrno(RegisterFiles, unsafe.Pointer(&files[0]), uint32(len(files)))
		if ret > 0 {
			break
		}
		if errno == syscall.EMFILE && !didIncrease {
			didIncrease = true
			err = increaseRlimitNoFile(uint64(len(files)))
			if err != nil {
				break
			}
			continue
		}
		break
	}
	return ret, err
}

func (ring *Ring) UnregisterFiles() (uint, error) {
	return ring.doRegister(UnregisterFiles, unsafe.Pointer(nil), 0)
}

func (ring *Ring) RegisterEventFd(fd int) (uint, error) {
	return ring.doRegister(RegisterEventFd, unsafe.Pointer(&fd), 1)
}

func (ring *Ring) UnregisterEventFd(fd int) (uint, error) {
	return ring.doRegister(UnregisterEventFd, unsafe.Pointer(&fd), 1)
}

func (ring *Ring) RegisterEventFdAsync(fd int) (uint, error) {
	return ring.doRegister(RegisterEventFDAsync, unsafe.Pointer(&fd), 1)
}

func (ring *Ring) RegisterProbe(probe *Probe, nrOps int) (uint, error) {
	result, err := ring.doRegister(RegisterProbe, unsafe.Pointer(probe), uint32(nrOps))
	runtime.KeepAlive(probe)

	return result, err
}

func (ring *Ring) RegisterPersonality() (uint, error) {
	return ring.doRegister(RegisterPersonality, unsafe.Pointer(nil), 0)
}

func (ring *Ring) UnregisterPersonality() (uint, error) {
	return ring.doRegister(UnregisterPersonality, unsafe.Pointer(nil), 0)
}

func (ring *Ring) RegisterRestrictions(res []Restriction) (uint, error) {
	return ring.doRegister(RegisterRestrictions, unsafe.Pointer(&res[0]), uint32(len(res)))
}

func (ring *Ring) doRegisterErrno(opCode uint32, arg unsafe.Pointer, nrArgs uint32) (uint, syscall.Errno) {
	var fd int
	if ring.kind&regRing != 0 {
		opCode |= RegisterUseRegisteredRing
		fd = ring.enterRingFd
	} else {
		fd = ring.ringFd
	}
	return ring.Register(fd, opCode, arg, nrArgs)
}

func (ring *Ring) RegisterIOWQAff(cpusz uint64, mask *unix.CPUSet) error {
	if cpusz >= 1<<31 {
		return syscall.EINVAL
	}
	_, err := ring.doRegister(RegisterIOWQAff, unsafe.Pointer(mask), uint32(cpusz))
	runtime.KeepAlive(mask)
	return err
}

func (ring *Ring) UnregisterIOWQAff() (uint, error) {
	return ring.doRegister(UnregisterIOWQAff, unsafe.Pointer(nil), 0)
}

const (
	IOWQBound uint = iota
	IOWQUnbound
)

const regIOWQMaxWorkersNrArgs = 2

func (ring *Ring) RegisterIOWQMaxWorkers(val []uint) (uint, error) {
	return ring.doRegister(RegisterIOWQMaxWorkers, unsafe.Pointer(&val[0]), regIOWQMaxWorkersNrArgs)
}

const registerRingFdOffset = uint32(4294967295)

func (ring *Ring) RegisterRingFd() (uint, error) {
	if (ring.kind & regRing) != 0 {
		return 0, syscall.EEXIST
	}
	update := &RsrcUpdate{
		Data:   uint64(ring.ringFd),
		Offset: registerRingFdOffset,
	}
	ret, err := ring.doRegister(RegisterRingFDs, unsafe.Pointer(update), 1)
	if err != nil {
		return ret, err
	}
	if ret == 1 {
		ring.enterRingFd = int(update.Offset)
		ring.kind |= regRing

		if ring.features&FeatRegRegRing != 0 {
			ring.kind |= doubleRegRing
		}
	} else {
		return ret, fmt.Errorf("unexpected return from ring.Register: %d", ret)
	}
	return ret, nil
}

func (ring *Ring) UnregisterRingFd() (uint, error) {
	update := &RsrcUpdate{
		Offset: uint32(ring.enterRingFd),
	}
	if (ring.kind & regRing) != 0 {
		return 0, syscall.EINVAL
	}
	ret, err := ring.doRegister(UnregisterRingFDs, unsafe.Pointer(update), 1)
	if err != nil {
		return ret, err
	}
	if ret == 1 {
		ring.enterRingFd = ring.ringFd
		ring.kind &= ^(regRing | doubleRegRing)
	}
	return ret, nil
}

func (ring *Ring) RegisterBufferRing(reg *BufReg, _ uint32) (uint, error) {
	result, err := ring.doRegister(RegisterPbufRing, unsafe.Pointer(reg), 1)
	runtime.KeepAlive(reg)
	return result, err
}

func (ring *Ring) UnregisterBufferRing(bgid int) (uint, error) {
	reg := &BufReg{
		Bgid: uint16(bgid),
	}
	result, err := ring.doRegister(UnregisterPbufRing, unsafe.Pointer(reg), 1)
	runtime.KeepAlive(reg)
	return result, err
}

func (ring *Ring) RegisterSyncCancel(reg *SyncCancelReg) (uint, error) {
	return ring.doRegister(RegisterSyncCancel, unsafe.Pointer(reg), 1)
}

func (ring *Ring) RegisterFileAllocRange(off, length uint32) (uint, error) {
	fileRange := &FileIndexRange{
		Off: off,
		Len: length,
	}
	result, err := ring.doRegister(RegisterFileAllocRange, unsafe.Pointer(fileRange), 0)
	runtime.KeepAlive(fileRange)
	return result, err
}

func (ring *Ring) doRegister(opCode uint32, arg unsafe.Pointer, nrArgs uint32) (uint, error) {
	ret, errno := ring.doRegisterErrno(opCode, arg, nrArgs)
	if errno != 0 {
		return 0, os.NewSyscallError("io_uring_register", errno)
	}
	return ret, nil
}

func increaseRlimitNoFile(nr uint64) error {
	limit := syscall.Rlimit{}
	err := syscall.Getrlimit(unix.RLIMIT_NOFILE, &limit)
	if err != nil {
		return err
	}
	if limit.Cur < nr {
		limit.Cur += nr
		err = syscall.Setrlimit(unix.RLIMIT_NOFILE, &limit)
		if err != nil {
			return err
		}
	}
	return nil
}
