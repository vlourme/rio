//go:build linux

package aio

import (
	"fmt"
	"runtime"
)

func (engine *Engine) Start() {
	// cpu num
	cpuNum := runtime.NumCPU() * 2
	settings := ResolveSettings[IOURingSettings](engine.settings)
	// entries
	entries := settings.Entries
	if entries == 0 {
		entries = uint32(cpuNum * 1024)
	}
	// setup
	fd := 0
	var fdErr error
	if settings.Param == nil {
		fd, fdErr = setupRing(entries, settings.Flags)
	} else if settings.Param != nil {
		if settings.AppMemBuf == nil {
			fd, fdErr = setupRingWithParam(entries, settings.Param)
		} else {
			fd, fdErr = setupRingWithParamAndAppMem(entries, settings.Param, settings.AppMemBuf)
		}
	}
	if fdErr != nil {
		panic(fmt.Errorf("aio: engine start failed, %v", fdErr))
		return
	}

	engine.fd = fd

	// loop

}

func (engine *Engine) Stop() {

}

const (
	SetupIOPoll uint32 = 1 << iota
	SetupSQPoll
	SetupSQAff
	SetupCQSize
	SetupClamp
	SetupAttachWQ
	SetupRDisabled
	SetupSubmitAll
	SetupCoopTaskrun
	SetupTaskrunFlag
	SetupSQE128
	SetupCQE32
	SetupSingleIssuer
	SetupDeferTaskrun
	SetupNoMmap
	SetupRegisteredFdOnly
)

func setupRing(entries uint32, flags uint32) (fd int, err error) {
	fd, err = setupRingWithParam(entries, &IOURingSetupParam{
		Flags: flags,
	})
	return
}

func setupRingWithParam(entries uint32, param *IOURingSetupParam) (fd int, err error) {
	fd, err = setupRingWithParamAndAppMem(entries, param, nil)
	return
}

func setupRingWithParamAndAppMem(entries uint32, param *IOURingSetupParam, mem *IOURingMem) (fd int, err error) {
	//var sqEntries, index uint32
	//var err error
	//
	//if param.Flags&SetupRegisteredFdOnly != 0 && param.Flags&SetupNoMmap == 0 {
	//	err = syscall.EINVAL
	//	return
	//}
	//
	//if param.Flags&SetupNoMmap != 0 {
	//	_, err = allocHuge(entries, p, ring.sqRing, ring.cqRing, buf, bufSize)
	//	if err != nil {
	//		return err
	//	}
	//	if buf != nil {
	//		ring.intFlags |= IntFlagAppMem
	//	}
	//}
	return
}

type IOURingSettings struct {
	Entries   uint32
	Flags     uint32
	Param     *IOURingSetupParam
	AppMemBuf *IOURingMem
}

type IOURingMem struct {
	Buf *byte
	Len uint64
}

type IOURingSetupParam struct {
	SQEntries    uint32
	CQEntries    uint32
	Flags        uint32
	SQThreadCPU  uint32
	SQThreadIdle uint32
	Features     uint32
	WqFd         uint32
	Resv         [3]uint32
	SQOff        SQRingOffsets
	CQOff        CQRingOffsets
}

type SQRingOffsets struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Flags       uint32
	Dropped     uint32
	Array       uint32
	Resv1       uint32
	UserAddr    uint64
}

type CQRingOffsets struct {
	Head        uint32
	Tail        uint32
	RingMask    uint32
	RingEntries uint32
	Overflow    uint32
	CQes        uint32
	Flags       uint32
	Resv1       uint32
	UserAddr    uint64
}
