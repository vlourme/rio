//go:build linux

package iouring

import "unsafe"

const (
	CQEFBuffer uint32 = 1 << iota
	CQEFMore
	CQEFSockNonempty
	CQEFNotify
)

const CQEBufferShift uint32 = 16

const CQEventFdDisabled uint32 = 1 << 0

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
