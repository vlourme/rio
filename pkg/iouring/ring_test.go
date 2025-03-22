//go:build linux

package iouring_test

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestNew(t *testing.T) {
	ring, ringErr := iouring.New(iouring.WithEntries(4))
	if ringErr != nil {
		t.Error(ringErr)
		return
	}
	defer ring.Close()

	t.Log("sq:", ring.SQEntries())
	t.Log("cq:", ring.CQEntries())

	sq := ring.GetSQE()
	if sq == nil {
		t.Error("SQE is nil")
		return
	}
	sq.PrepareNop()
	sq.SetData64(1)

	n, subErr := ring.Submit()
	if subErr != nil {
		t.Error(subErr)
		return
	}
	t.Log("sub:", n)

	cqe, waitErr := ring.WaitCQE()
	if waitErr != nil {
		t.Error(waitErr)
		return
	}
	if cqe.UserData == sq.UserData {
		t.Log("succeed")
	} else {
		t.Error("UserData not equal")
	}
}

func TestSubmissionQueueEntry_PrepareLinkTimeout(t *testing.T) {
	ring, ringErr := iouring.New(
		iouring.WithEntries(4),
		iouring.WithFlags(iouring.SetupSQPoll),
	)
	if ringErr != nil {
		t.Error(ringErr)
		return
	}
	defer ring.Close()

	t.Log("sq:", ring.SQEntries())
	t.Log("cq:", ring.CQEntries())

	var pipe [2]int
	if err := syscall.Pipe2(pipe[:], syscall.O_CLOEXEC|syscall.O_NONBLOCK); err != nil {
		t.Error(err)
		return
	}
	defer syscall.Close(pipe[0])
	defer syscall.Close(pipe[1])

	// timeout: timeout_sqe (ETIME) op_sqe (ECANCELED)
	// SUCCEED: timeout_sqe (ECANCELED) op_sqe (OK)
	// DONT CANCEL OP AFTER timeout
	// will get at same time, so wait twice and send twice when op has deadline
	// todo use tagged ptr to check which is timeout cqe
	//_, _ = syscall.Write(pipe[1], []byte{1})

	go func() {
		time.Sleep(1 * time.Second)
		//_, _ = syscall.Write(pipe[1], []byte{1})
		//syscall.Close(pipe[0])
		//syscall.Close(pipe[1])
	}()

	readSQE := ring.GetSQE()
	b := make([]byte, 1)
	readSQE.PrepareRead(pipe[0], uintptr(unsafe.Pointer(&b[0])), 1, 0)
	readSQE.SetData64(1)
	readSQE.Flags = iouring.SQEIOLink

	timeoutSQE := ring.GetSQE()
	ns := syscall.NsecToTimespec((500 * time.Millisecond).Nanoseconds())
	timeoutSQE.PrepareLinkTimeout(&ns, 0)
	timeoutSQE.SetData64(2)

	t.Log(ring.Submit())
	//_, _ = ring.Submit()
	ns2 := syscall.NsecToTimespec(3000 * time.Millisecond.Nanoseconds())
	cqe0, cqe0Err := ring.WaitCQETimeout(&ns2)
	if cqe0Err != nil {
		t.Error(cqe0Err)
		return
	}
	t.Log("cqes:", ring.CQReady())
	ring.CQESeen(cqe0)
	if cqe0.Res < 0 {
		t.Log("c0:", cqe0.Res, syscall.Errno(-cqe0.Res), int(syscall.ETIME), int(syscall.ECANCELED), cqe0.Flags, cqe0.UserData)
	} else {
		t.Log("c0:", cqe0.Res, cqe0.Flags, cqe0.UserData)
	}

	for i := 0; i < 1; i++ {
		cqe, cqeErr := ring.WaitCQE()
		if cqeErr != nil {
			t.Error(cqeErr)
			return
		}
		if cqe.Res < 0 {
			t.Log("cqe:", cqe.Res, syscall.Errno(-cqe.Res), int(syscall.ETIME), int(syscall.ECANCELED), cqe.Flags, cqe.UserData)
		} else {
			t.Log("cqe:", cqe.Res, cqe.Flags, cqe.UserData)
		}
		ring.CQESeen(cqe)
	}

}
