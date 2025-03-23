//go:build linux

package iouring

import (
	"golang.org/x/sys/unix"
	"math"
	"syscall"
	"unsafe"
)

const (
	OpNop uint8 = iota
	OpReadv
	OpWritev
	OpFsync
	OpReadFixed
	OpWriteFixed
	OpPollAdd
	OpPollRemove
	OpSyncFileRange
	OpSendmsg
	OpRecvmsg
	OpTimeout
	OpTimeoutRemove
	OpAccept
	OpAsyncCancel
	OpLinkTimeout
	OpConnect
	OpFallocate
	OpOpenat
	OpClose
	OpFilesUpdate
	OpStatx
	OpRead
	OpWrite
	OpFadvise
	OpMadvise
	OpSend
	OpRecv
	OpOpenat2
	OpEpollCtl
	OpSplice
	OpProvideBuffers
	OpRemoveBuffers
	OpTee
	OpShutdown
	OpRenameat
	OpUnlinkat
	OpMkdirat
	OpSymlinkat
	OpLinkat
	OpMsgRing
	OpFsetxattr
	OpSetxattr
	OpFgetxattr
	OpGetxattr
	OpSocket
	OpUringCmd
	OpSendZC
	OpSendMsgZC
	OpReadMultishot
	OpWaitId
	OpFutexWait
	OpFutexWake
	OpFutexWaitv
	OPFixedFdInstall
	OpFtuncate
	OpBind
	OpListen
	OpRecvZC
	OpEpollWait
	OpReadvFixed
	OpWritevFixed

	OpLast = math.MaxUint8
)

const UringCmdFixed uint32 = 1 << 0

const FsyncDatasync uint32 = 1 << 0

const (
	TimeoutAbs uint32 = 1 << iota
	TimeoutUpdate
	TimeoutBoottime
	TimeoutRealtime
	LinkTimeoutUpdate
	TimeoutETimeSuccess
	TimeoutMultishot
	TimeoutClockMask  = TimeoutBoottime | TimeoutRealtime
	TimeoutUpdateMask = TimeoutUpdate | LinkTimeoutUpdate
)

const SpliceFFdInFixed uint32 = 1 << 31

const (
	PollAddMulti uint32 = 1 << iota
	PollUpdateEvents
	PollUpdateUserData
	PollAddLevel
)

const (
	AsyncCancelAll uint32 = 1 << iota
	AsyncCancelFd
	AsyncCancelAny
	AsyncCancelFdFixed
)

const (
	RecvsendPollFirst uint16 = 1 << iota
	RecvMultishot
	RecvsendFixedBuf
	SendZCReportUsage
)

const NotifyUsageZCCopied uint32 = 1 << 31

const (
	AcceptMultishot uint16 = 1 << iota
)

const (
	MsgData uint32 = iota
	MsgSendFd
)

var msgDataVar = MsgData

const (
	MsgRingCQESkip uint32 = 1 << iota
	MsgRingFlagsPass
)

const FileIndexAlloc uint32 = 4294967295

const (
	SocketOpSIOCInQ = iota
	SocketOpSIOCOutQ
	SocketOpGetsockopt
	SocketOpSetsockopt
)

const (
	SQEFixedFile uint8 = 1 << iota
	SQEIODrain
	SQEIOLink
	SQEIOHardlink
	SQEAsync
	SQEBufferSelect
	SQECQESkipSuccess
)

const (
	FixedFdNoCloexec uint32 = 1 << iota
)

type union64_2 struct {
	n1 uint32
	n2 uint32
}

func mergeUint32ToUint64(n1 uint32, n2 uint32) uint64 {
	n := union64_2{n1, n2}
	np := unsafe.Pointer(&n)
	return uint64(*(*uint64)(np))
}

type SubmissionQueueEntry struct {
	OpCode      uint8
	Flags       uint8
	IoPrio      uint16
	Fd          int32
	Off         uint64
	Addr        uint64
	Len         uint32
	OpcodeFlags uint32
	UserData    uint64
	BufIG       uint16
	Personality uint16
	SpliceFdIn  int32
	Addr3       uint64
	_pad2       [1]uint64
}

func (entry *SubmissionQueueEntry) SetData(data unsafe.Pointer) {
	entry.UserData = uint64(uintptr(data))
}

func (entry *SubmissionQueueEntry) SetData64(data uint64) {
	entry.UserData = data
}

func (entry *SubmissionQueueEntry) SetFlags(flags uint8) {
	entry.Flags |= flags
}

// [Nop] ***************************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareNop() {
	entry.prepareRW(OpNop, -1, 0, 0, 0)
}

// [Net] ***************************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareSetsockoptInt(fd int, level int, optName int, optValue *int) {
	entry.prepareRW(OpUringCmd, fd, 0, 0, 0)
	entry.Addr3 = uint64(uintptr(unsafe.Pointer(optValue)))          // optval
	entry.Addr = mergeUint32ToUint64(uint32(level), uint32(optName)) // level, optname
	entry.SpliceFdIn = int32(unsafe.Sizeof(optValue))                // optlen
	entry.Off = SocketOpSetsockopt                                   // cmd_op
}

func (entry *SubmissionQueueEntry) PrepareGetsockoptInt(fd int, level int, optName int, optValue *int) {
	entry.prepareRW(OpUringCmd, fd, 0, 0, 0)
	entry.Addr3 = uint64(uintptr(unsafe.Pointer(optValue)))          // optval
	entry.Addr = mergeUint32ToUint64(uint32(level), uint32(optName)) // level, optname
	optValueLen := unsafe.Sizeof(optValue)
	entry.SpliceFdIn = int32(unsafe.Sizeof(unsafe.Pointer(&optValueLen))) // optlen
	entry.Off = SocketOpGetsockopt                                        // cmd_op
}

func (entry *SubmissionQueueEntry) PrepareBind(fd int, addr *syscall.RawSockaddrAny, addrLen uint64) {
	entry.prepareRW(OpBind, fd, uintptr(unsafe.Pointer(addr)), 0, addrLen)
}

func (entry *SubmissionQueueEntry) PrepareListen(fd int, backlog uint32) {
	entry.prepareRW(OpListen, fd, 0, backlog, 0)
}

func (entry *SubmissionQueueEntry) PrepareAccept(fd int, addr *syscall.RawSockaddrAny, addrLen uint64, flags int) {
	entry.prepareRW(OpAccept, fd, uintptr(unsafe.Pointer(addr)), 0, addrLen)
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareAcceptDirect(fd int, addr *syscall.RawSockaddrAny, addrLen uint64, flags int, fileIndex uint32) {
	entry.PrepareAccept(fd, addr, addrLen, flags)
	if fileIndex == FileIndexAlloc {
		fileIndex--
	}
	entry.setTargetFixedFile(fileIndex)
}

func (entry *SubmissionQueueEntry) PrepareAcceptDirectAlloc(fd int, addr *syscall.RawSockaddrAny, addrLen uint64, flags int) {
	entry.PrepareAccept(fd, addr, addrLen, flags)
	entry.setTargetFixedFile(FileIndexAlloc - 1)
}

func (entry *SubmissionQueueEntry) PrepareAcceptMultishot(fd int, addr *syscall.RawSockaddrAny, addrLen uint64, flags int) {
	entry.PrepareAccept(fd, addr, addrLen, flags)
	entry.IoPrio |= AcceptMultishot
}

func (entry *SubmissionQueueEntry) PrepareAcceptMultishotDirect(fd int, addr *syscall.RawSockaddrAny, addrLen uint64, flags int) {
	entry.PrepareAcceptMultishot(fd, addr, addrLen, flags)
	entry.setTargetFixedFile(FileIndexAlloc - 1)
}

func (entry *SubmissionQueueEntry) PrepareConnect(fd int, addr *syscall.RawSockaddrAny, addrLen uint64) {
	entry.prepareRW(OpConnect, fd, uintptr(unsafe.Pointer(addr)), 0, addrLen)
}

func (entry *SubmissionQueueEntry) PrepareRecv(fd int, buf uintptr, length uint32, flags int) {
	entry.prepareRW(OpRecv, fd, buf, length, 0)
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareRecvMultishot(fd int, addr uintptr, length uint32, flags int) {
	entry.PrepareRecv(fd, addr, length, flags)
	entry.IoPrio |= RecvMultishot
}

func (entry *SubmissionQueueEntry) PrepareRecvMsg(fd int, msg *syscall.Msghdr, flags uint32) {
	entry.prepareRW(OpRecvmsg, fd, uintptr(unsafe.Pointer(msg)), 1, 0)
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareRecvMsgMultishot(fd int, msg *syscall.Msghdr, flags uint32) {
	entry.PrepareRecvMsg(fd, msg, flags)
	entry.IoPrio |= RecvMultishot
}

func (entry *SubmissionQueueEntry) PrepareSend(fd int, addr uintptr, length uint32, flags int) {
	entry.prepareRW(OpSend, fd, addr, length, 0)
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareSendZC(sockFd int, addr uintptr, length uint32, flags int, zcFlags uint32) {
	entry.prepareRW(OpSendZC, sockFd, addr, length, 0)
	entry.OpcodeFlags = uint32(flags)
	entry.IoPrio = uint16(zcFlags)
}

func (entry *SubmissionQueueEntry) PrepareSendZCFixed(sockFd int, addr uintptr, length uint32, flags int, zcFlags, bufIndex uint32) {
	entry.PrepareSendZC(sockFd, addr, length, flags, zcFlags)
	entry.IoPrio |= RecvsendFixedBuf
	entry.BufIG = uint16(bufIndex)
}

func (entry *SubmissionQueueEntry) PrepareSendMsg(fd int, msg *syscall.Msghdr, flags uint32) {
	entry.prepareRW(OpSendmsg, fd, uintptr(unsafe.Pointer(msg)), 1, 0)
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareSendmsgZC(fd int, msg *syscall.Msghdr, flags uint32) {
	entry.PrepareSendMsg(fd, msg, flags)
	entry.OpCode = OpSendMsgZC
}

func (entry *SubmissionQueueEntry) PrepareShutdown(fd, how int) {
	entry.prepareRW(OpShutdown, fd, 0, uint32(how), 0)
}

func (entry *SubmissionQueueEntry) PrepareSocket(domain, socketType, protocol int, flags uint32) {
	entry.prepareRW(OpSocket, domain, 0, uint32(protocol), uint64(socketType))
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareSocketDirect(domain, socketType, protocol int, fileIndex, flags uint32) {
	entry.PrepareSocket(domain, socketType, protocol, flags)
	if fileIndex == FileIndexAlloc {
		fileIndex--
	}
	entry.setTargetFixedFile(fileIndex)
}

func (entry *SubmissionQueueEntry) PrepareSocketDirectAlloc(domain, socketType, protocol int, flags uint32) {
	entry.PrepareSocket(domain, socketType, protocol, flags)
	entry.setTargetFixedFile(FileIndexAlloc - 1)
}

func (entry *SubmissionQueueEntry) PrepareSplice(fdIn int, offIn int64, fdOut int, offOut int64, nbytes, spliceFlags uint32) {
	entry.prepareRW(OpSplice, fdOut, 0, nbytes, uint64(offOut))
	entry.Addr = uint64(offIn)
	entry.SpliceFdIn = int32(fdIn)
	entry.OpcodeFlags = spliceFlags
}

func (entry *SubmissionQueueEntry) PrepareTee(fdIn, fdOut int, nbytes, spliceFlags uint32) {
	entry.prepareRW(OpTee, fdOut, 0, nbytes, 0)
	entry.Addr = 0
	entry.SpliceFdIn = int32(fdIn)
	entry.OpcodeFlags = spliceFlags
}

func RecvmsgValidate(buf unsafe.Pointer, bufLen int, msgh *syscall.Msghdr) *RecvmsgOut {
	header := uintptr(msgh.Controllen) + uintptr(msgh.Namelen) + unsafe.Sizeof(RecvmsgOut{})
	if bufLen < 0 || uintptr(bufLen) < header {
		return nil
	}
	return (*RecvmsgOut)(buf)
}

type RecvmsgOut struct {
	Namelen    uint32
	ControlLen uint32
	PayloadLen uint32
	Flags      uint32
}

func (o *RecvmsgOut) CmsgNexthdr(msgh *syscall.Msghdr, cmsg *syscall.Cmsghdr) *syscall.Cmsghdr {
	if cmsg.Len < syscall.SizeofCmsghdr {
		return nil
	}
	end := (*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(o.CmsgFirsthdr(msgh))) + uintptr(o.ControlLen)))
	cmsg = (*syscall.Cmsghdr)(unsafe.Pointer(uintptr(unsafe.Pointer(cmsg)) + uintptr(cmsgAlign(cmsg.Len))))
	if uintptr(unsafe.Pointer(cmsg))+unsafe.Sizeof(*cmsg) > uintptr(unsafe.Pointer(end)) {
		return nil
	}
	if uintptr(unsafe.Pointer(cmsg))+uintptr(cmsgAlign(cmsg.Len)) > uintptr(unsafe.Pointer(end)) {
		return nil
	}

	return cmsg
}

func (o *RecvmsgOut) Name() unsafe.Pointer {
	return unsafe.Pointer(uintptr(unsafe.Pointer(o)) + unsafe.Sizeof(*o))
}

func (o *RecvmsgOut) CmsgFirsthdr(msgh *syscall.Msghdr) *syscall.Cmsghdr {
	if o.ControlLen < syscall.SizeofCmsghdr {
		return nil
	}

	return (*syscall.Cmsghdr)(unsafe.Pointer(uintptr(o.Name()) + uintptr(msgh.Namelen)))
}

func (o *RecvmsgOut) Payload(msgh *syscall.Msghdr) unsafe.Pointer {
	return unsafe.Pointer(uintptr(unsafe.Pointer(o)) +
		unsafe.Sizeof(*o) +
		uintptr(msgh.Namelen) +
		uintptr(msgh.Controllen))
}

func (o *RecvmsgOut) PayloadLength(bufLen int, msgh *syscall.Msghdr) uint32 {
	payloadStart := uintptr(o.Payload(msgh))
	payloadEnd := uintptr(unsafe.Pointer(o)) + uintptr(bufLen)

	return uint32(payloadEnd - payloadStart)
}

func cmsgAlign(length uint64) uint64 {
	return (length + uint64(unsafe.Sizeof(uintptr(0))) - 1) & ^(uint64(unsafe.Sizeof(uintptr(0))) - 1)
}

// [CancelOperation] ************************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareCancel(userdata uintptr, flags uint32) {
	entry.PrepareCancel64(uint64(userdata), flags)
}

func (entry *SubmissionQueueEntry) PrepareCancel64(userdata uint64, flags uint32) {
	entry.prepareRW(OpAsyncCancel, -1, 0, 0, 0)
	entry.Addr = userdata
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareCancelFd(fd int, flags uint32) {
	entry.prepareRW(OpAsyncCancel, fd, 0, 0, 0)
	entry.OpcodeFlags = flags | AsyncCancelFd
}

func (entry *SubmissionQueueEntry) PrepareCancelFdFixed(fileIndex uint32, flags uint32) {
	entry.prepareRW(OpAsyncCancel, int(fileIndex), 0, 0, 0)
	entry.OpcodeFlags = flags | AsyncCancelFd | AsyncCancelFdFixed
}

func (entry *SubmissionQueueEntry) PrepareCancelALL() {
	entry.prepareRW(OpAsyncCancel, 0, 0, 0, 0)
	entry.OpcodeFlags = AsyncCancelAll
}

// [Timeout] ***********************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareLinkTimeout(spec *syscall.Timespec, flags uint32) {
	entry.prepareRW(OpLinkTimeout, -1, uintptr(unsafe.Pointer(spec)), 1, 0)
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareTimeout(spec *syscall.Timespec, count, flags uint32) {
	entry.prepareRW(OpTimeout, -1, uintptr(unsafe.Pointer(&spec)), 1, uint64(count))
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareTimeoutRemove(spec *syscall.Timespec, count uint64, flags uint32) {
	entry.prepareRW(OpTimeoutRemove, -1, uintptr(unsafe.Pointer(spec)), 1, count)
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareTimeoutUpdate(spec *syscall.Timespec, count uint64, flags uint32) {
	entry.prepareRW(OpTimeoutRemove, -1, uintptr(unsafe.Pointer(spec)), 1, count)
	entry.OpcodeFlags = flags | TimeoutUpdate
}

// [Close] *************************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareClose(fd int) {
	entry.prepareRW(OpClose, fd, 0, 0, 0)
}

func (entry *SubmissionQueueEntry) PrepareCloseDirect(fileIndex uint32) {
	entry.PrepareClose(0)
	entry.setTargetFixedFile(fileIndex)
}

// [MsgRing] ***********************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareMsgRing(fd int, length uint32, data uint64, flags uint32) {
	entry.prepareRW(OpMsgRing, fd, 0, length, data)
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareMsgRingCqeFlags(fd int, length uint32, data uint64, flags, cqeFlags uint32) {
	entry.prepareRW(OpMsgRing, fd, 0, length, data)
	entry.OpcodeFlags = MsgRingFlagsPass | flags
	entry.SpliceFdIn = int32(cqeFlags)
}

func (entry *SubmissionQueueEntry) PrepareMsgRingFd(fd int, sourceFd int, targetFd int, data uint64, flags uint32) {
	entry.prepareRW(OpMsgRing, fd, uintptr(unsafe.Pointer(&msgDataVar)), 0, data)
	entry.Addr3 = uint64(sourceFd)
	if uint32(targetFd) == FileIndexAlloc {
		targetFd--
	}
	entry.setTargetFixedFile(uint32(targetFd))
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareMsgRingFdAlloc(fd int, sourceFd int, data uint64, flags uint32) {
	entry.PrepareMsgRingFd(fd, sourceFd, int(FileIndexAlloc), data, flags)
}

// [File] **************************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareOpenat(dfd int, path []byte, flags int, mode uint32) {
	entry.prepareRW(OpOpenat, dfd, uintptr(unsafe.Pointer(&path)), mode, 0)
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareOpenat2(dfd int, path []byte, openHow *unix.OpenHow) {
	entry.prepareRW(OpOpenat, dfd, uintptr(unsafe.Pointer(&path)),
		uint32(unsafe.Sizeof(*openHow)), uint64(uintptr(unsafe.Pointer(openHow))))
}

func (entry *SubmissionQueueEntry) PrepareOpenat2Direct(dfd int, path []byte, openHow *unix.OpenHow, fileIndex uint32) {
	entry.PrepareOpenat2(dfd, path, openHow)
	if fileIndex == FileIndexAlloc {
		fileIndex--
	}
	entry.setTargetFixedFile(fileIndex)
}

func (entry *SubmissionQueueEntry) PrepareOpenatDirect(dfd int, path []byte, flags int, mode uint32, fileIndex uint32) {
	entry.PrepareOpenat(dfd, path, flags, mode)
	if fileIndex == FileIndexAlloc {
		fileIndex--
	}
	entry.setTargetFixedFile(fileIndex)
}

func (entry *SubmissionQueueEntry) PrepareRead(fd int, buf uintptr, nbytes uint32, offset uint64) {
	entry.prepareRW(OpRead, fd, buf, nbytes, offset)
}

func (entry *SubmissionQueueEntry) PrepareReadFixed(fd int, buf uintptr, nbytes uint32, offset uint64, bufIndex int) {
	entry.prepareRW(OpReadFixed, fd, buf, nbytes, offset)
	entry.BufIG = uint16(bufIndex)
}

func (entry *SubmissionQueueEntry) PrepareReadv(fd int, iovecs uintptr, nrVecs uint32, offset uint64) {
	entry.prepareRW(OpReadv, fd, iovecs, nrVecs, offset)
}

func (entry *SubmissionQueueEntry) PrepareReadv2(fd int, iovecs uintptr, nrVecs uint32, offset uint64, flags int) {
	entry.PrepareReadv(fd, iovecs, nrVecs, offset)
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareWrite(fd int, buf uintptr, nbytes uint32, offset uint64) {
	entry.prepareRW(OpWrite, fd, buf, nbytes, offset)
}

func (entry *SubmissionQueueEntry) PrepareWriteFixed(
	fd int, vectors uintptr, length uint32, offset uint64, index int) {
	entry.prepareRW(OpWriteFixed, fd, vectors, length, offset)
	entry.BufIG = uint16(index)
}

func (entry *SubmissionQueueEntry) PrepareWritev(
	fd int, iovecs uintptr, nrVecs uint32, offset uint64) {
	entry.prepareRW(OpWritev, fd, iovecs, nrVecs, offset)
}

func (entry *SubmissionQueueEntry) PrepareWritev2(
	fd int, iovecs uintptr, nrVecs uint32, offset uint64, flags int) {
	entry.PrepareWritev(fd, iovecs, nrVecs, offset)
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareRename(oldPath, netPath []byte) {
	entry.PrepareRenameat(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, netPath, 0)
}

func (entry *SubmissionQueueEntry) PrepareRenameat(oldFd int, oldPath []byte, newFd int, newPath []byte, flags uint32) {
	entry.prepareRW(OpRenameat, oldFd,
		uintptr(unsafe.Pointer(&oldPath)), uint32(newFd), uint64(uintptr(unsafe.Pointer(&newPath))))
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareLink(oldPath, newPath []byte, flags int) {
	entry.PrepareLinkat(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, flags)
}

func (entry *SubmissionQueueEntry) PrepareLinkat(oldFd int, oldPath []byte, newFd int, newPath []byte, flags int) {
	entry.prepareRW(OpLinkat, oldFd, uintptr(unsafe.Pointer(&oldPath)),
		uint32(newFd), uint64(uintptr(unsafe.Pointer(&newPath))))
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareUnlink(path uintptr, flags int) {
	entry.PrepareUnlinkat(unix.AT_FDCWD, path, flags)
}

func (entry *SubmissionQueueEntry) PrepareUnlinkat(dfd int, path uintptr, flags int) {
	entry.prepareRW(OpUnlinkat, dfd, path, 0, 0)
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareMkdir(path []byte, mode uint32) {
	entry.PrepareMkdirat(unix.AT_FDCWD, path, mode)
}

func (entry *SubmissionQueueEntry) PrepareMkdirat(dfd int, path []byte, mode uint32) {
	entry.prepareRW(OpMkdirat, dfd, uintptr(unsafe.Pointer(&path)), mode, 0)
}

func (entry *SubmissionQueueEntry) PrepareFilesUpdate(fds []int, offset int) {
	entry.prepareRW(OpFilesUpdate, -1, uintptr(unsafe.Pointer(&fds)), uint32(len(fds)), uint64(offset))
}

func (entry *SubmissionQueueEntry) PrepareStatx(dfd int, path []byte, flags int, mask uint32, statx *unix.Statx_t) {
	entry.prepareRW(OpStatx, dfd, uintptr(unsafe.Pointer(&path)), mask, uint64(uintptr(unsafe.Pointer(statx))))
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareSymlink(target, linkpath []byte) {
	entry.PrepareSymlinkat(target, unix.AT_FDCWD, linkpath)
}

func (entry *SubmissionQueueEntry) PrepareSymlinkat(target []byte, newdirfd int, linkpath []byte) {
	entry.prepareRW(OpSymlinkat, newdirfd, uintptr(unsafe.Pointer(&target)), 0, uint64(uintptr(unsafe.Pointer(&linkpath))))
}

func (entry *SubmissionQueueEntry) PrepareSyncFileRange(fd int, length uint32, offset uint64, flags int) {
	entry.prepareRW(OpSyncFileRange, fd, 0, length, offset)
	entry.OpcodeFlags = uint32(flags)
}

// [F] *****************************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareFixedFdInstall(fd int, flags uint32) {
	entry.prepareRW(OPFixedFdInstall, fd, 0, 0, 0)
	entry.Flags = SQEFixedFile
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareFadvise(fd int, offset uint64, length int, advise uint32) {
	entry.prepareRW(OpFadvise, fd, 0, uint32(length), offset)
	entry.OpcodeFlags = advise
}

func (entry *SubmissionQueueEntry) PrepareFallocate(fd int, mode int, offset, length uint64) {
	entry.prepareRW(OpFallocate, fd, 0, uint32(mode), offset)
	entry.Addr = length
}

func (entry *SubmissionQueueEntry) PrepareFgetxattr(fd int, name, value []byte) {
	entry.prepareRW(OpFgetxattr, fd, uintptr(unsafe.Pointer(&name)),
		uint32(len(value)), uint64(uintptr(unsafe.Pointer(&value))))
}

func (entry *SubmissionQueueEntry) PrepareSetxattr(name, value, path []byte, flags int, length uint32) {
	entry.prepareRW(OpSetxattr, 0, uintptr(unsafe.Pointer(&name)), length, uint64(uintptr(unsafe.Pointer(&value))))
	entry.Addr3 = uint64(uintptr(unsafe.Pointer(&path)))
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareFsetxattr(fd int, name, value []byte, flags int) {
	entry.prepareRW(
		OpFsetxattr, fd, uintptr(unsafe.Pointer(&name)), uint32(len(value)), uint64(uintptr(unsafe.Pointer(&value))))
	entry.OpcodeFlags = uint32(flags)
}

func (entry *SubmissionQueueEntry) PrepareFsync(fd int, flags uint32) {
	entry.prepareRW(OpFsync, fd, 0, 0, 0)
	entry.OpcodeFlags = flags
}

func (entry *SubmissionQueueEntry) PrepareGetxattr(name, value, path []byte) {
	entry.prepareRW(OpGetxattr, 0, uintptr(unsafe.Pointer(&name)),
		uint32(len(value)), uint64(uintptr(unsafe.Pointer(&value))))
	entry.Addr3 = uint64(uintptr(unsafe.Pointer(&path)))
	entry.OpcodeFlags = 0
}

func (entry *SubmissionQueueEntry) PrepareMadvise(addr uintptr, length uint, advice int) {
	entry.prepareRW(OpMadvise, -1, addr, uint32(length), 0)
	entry.OpcodeFlags = uint32(advice)
}

// [Kernel Buffer] *****************************************************************************************************

func (entry *SubmissionQueueEntry) PrepareProvideBuffers(addr uintptr, length, nr, bgid, bid int) {
	entry.prepareRW(OpProvideBuffers, nr, addr, uint32(length), uint64(bid))
	entry.BufIG = uint16(bgid)
}

func (entry *SubmissionQueueEntry) PrepareRemoveBuffers(nr int, bgid int) {
	entry.prepareRW(OpRemoveBuffers, nr, 0, 0, 0)
	entry.BufIG = uint16(bgid)
}

// [Poll] **************************************************************************************************************

func (entry *SubmissionQueueEntry) PreparePollAdd(fd int, pollMask uint32) {
	entry.prepareRW(OpPollAdd, fd, 0, 0, 0)
	entry.OpcodeFlags = pollMask
}

func (entry *SubmissionQueueEntry) PreparePollMultishot(fd int, pollMask uint32) {
	entry.PreparePollAdd(fd, pollMask)
	entry.Len = PollAddMulti
}

func (entry *SubmissionQueueEntry) PreparePollRemove(userData uint64) {
	entry.prepareRW(OpPollRemove, -1, 0, 0, 0)
	entry.Addr = userData
}

func (entry *SubmissionQueueEntry) PreparePollUpdate(oldUserData, newUserData uint64, pollMask, flags uint32) {
	entry.prepareRW(OpPollRemove, -1, 0, flags, newUserData)
	entry.Addr = oldUserData
	entry.OpcodeFlags = pollMask
}

func (entry *SubmissionQueueEntry) PrepareEPollWait(fd int, events []syscall.EpollEvent, flags uint32) {
	addr := unsafe.Pointer(&events[0])
	length := uint32(len(events))
	entry.prepareRW(OpEpollWait, fd, uintptr(addr), length, 0)
	entry.OpcodeFlags = flags
}

// [private] ***********************************************************************************************************

func (entry *SubmissionQueueEntry) prepareRW(opcode uint8, fd int, addr uintptr, length uint32, offset uint64) {
	entry.OpCode = opcode
	entry.Flags = 0
	entry.IoPrio = 0
	entry.Fd = int32(fd)
	entry.Off = offset
	entry.Addr = uint64(addr)
	entry.Len = length
	entry.UserData = 0
	entry.BufIG = 0
	entry.Personality = 0
	entry.SpliceFdIn = 0
}

func (entry *SubmissionQueueEntry) setTargetFixedFile(fileIndex uint32) {
	entry.SpliceFdIn = int32(fileIndex + 1)
}

const (
	SQNeedWakeup uint32 = 1 << iota
	SQCQOverflow
	SQTaskRun
)

type SQRingOffsets struct {
	head        uint32
	tail        uint32
	ringMask    uint32
	ringEntries uint32
	flags       uint32
	dropped     uint32
	array       uint32
	resv1       uint32
	userAddr    uint64
}

type SubmissionQueue struct {
	head        *uint32
	tail        *uint32
	ringMask    *uint32
	ringEntries *uint32
	flags       *uint32
	dropped     *uint32
	array       *uint32
	sqes        *SubmissionQueueEntry
	ringSize    uint
	ringPtr     unsafe.Pointer
	sqeHead     uint32
	sqeTail     uint32
	pad         [2]uint32
}
