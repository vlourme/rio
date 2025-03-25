//go:build linux

package liburing_test

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"syscall"
	"testing"
)

func TestRing_RegisterFileAllocRange(t *testing.T) {
	t.Log("kernel:", liburing.GetVersion())

	ring, ringErr := liburing.New(liburing.WithEntries(4))
	if ringErr != nil {
		t.Error(ringErr)
		return
	}
	defer ring.Close()

	if _, regErr := ring.RegisterFilesSparse(65535); regErr != nil {
		t.Error(regErr)
		return
	}

	defer ring.UnregisterFiles()

	if _, regErr := ring.RegisterFileAllocRange(5, 65535-5); regErr != nil {
		t.Error(regErr)
		return
	}

	var (
		sqe    *liburing.SubmissionQueueEntry
		sn     uint
		sErr   error
		cqe    *liburing.CompletionQueueEvent
		cqeErr error
	)

	sqe = ring.GetSQE()
	sqe.PrepareSocket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK, 0, 0)
	sn, sErr = ring.SubmitAndWait(1)
	if sErr != nil {
		t.Error(sErr)
		return
	}
	t.Log("submitted:", sn)
	cqe, cqeErr = ring.PeekCQE()
	if cqeErr != nil {
		t.Error(cqeErr)
		return
	}
	t.Log("cqe:", cqe.Res, cqe.Flags)
	ring.CQAdvance(1)

	if _, regErr := ring.RegisterFilesUpdate(1, []int{int(cqe.Res), -1, -1, -1}); regErr != nil {
		t.Error(regErr)
		return
	}

	sqe = ring.GetSQE()
	sqe.PrepareSocket(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK, 0, 0)
	sn, sErr = ring.SubmitAndWait(1)
	if sErr != nil {
		t.Error(sErr)
		return
	}
	t.Log("submitted:", sn)
	cqe, cqeErr = ring.PeekCQE()
	if cqeErr != nil {
		t.Error(cqeErr)
		return
	}
	t.Log("cqe:", cqe.Res, cqe.Flags)
	ring.CQAdvance(1)

	if _, regErr := ring.RegisterFilesUpdate(2, []int{int(cqe.Res), -1, -1}); regErr != nil {
		t.Error(regErr)
		return
	}

	sqe = ring.GetSQE()
	sqe.PrepareSocketDirectAlloc(syscall.AF_INET, syscall.SOCK_STREAM|syscall.SOCK_NONBLOCK, 0, 0)
	sn, sErr = ring.SubmitAndWait(1)
	if sErr != nil {
		t.Error(sErr)
		return
	}
	cqe, cqeErr = ring.PeekCQE()
	if cqeErr != nil {
		t.Error(cqeErr)
		return
	}
	t.Log("cqe:", cqe.Res, cqe.Flags)
	ring.CQAdvance(1)

}
