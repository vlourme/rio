package iouring

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
	OpLast
)

const opSupported uint16 = 1 << 0
