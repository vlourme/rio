//go:build linux

package liburing

import "unsafe"

const (
	IORING_CQE_F_BUFFER uint32 = 1 << iota
	IORING_CQE_F_MORE
	IORING_CQE_F_SOCK_NONEMPTY
	IORING_CQE_F_NOTIF
	IORING_CQE_F_BUF_MORE
)

const IORING_CQE_BUFFER_SHIFT = 16

const IORING_CQ_EVENTFD_DISABLED uint32 = 1 << 0

type CompletionQueueEvent struct {
	UserData uint64
	Res      int32
	Flags    uint32
}

func (c *CompletionQueueEvent) GetData() unsafe.Pointer {
	if c.UserData == 0 {
		return nil
	}
	return unsafe.Pointer(uintptr(c.UserData))
}

func (c *CompletionQueueEvent) IsInternalUpdateTimeoutUserdata() bool {
	return c.UserData == _updateTimeoutUserdata
}

type CQRingOffsets struct {
	head        uint32
	tail        uint32
	ringMask    uint32
	ringEntries uint32
	overflow    uint32
	cqes        uint32
	flags       uint32
	resv1       uint32
	userAddr    uint64
}

type CompletionQueue struct {
	head        *uint32
	tail        *uint32
	ringMask    *uint32
	ringEntries *uint32
	flags       *uint32
	overflow    *uint32
	cqes        *CompletionQueueEvent
	ringSize    uint
	ringPtr     unsafe.Pointer
	pad         [2]uint32
}
