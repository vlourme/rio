//go:build linux

package iouring_test

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	ring, ringErr := iouring.New(iouring.WithEntries(4), iouring.WithFlags(iouring.SetupSQAff), iouring.WithSQThreadCPU(1))
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

func TestWithAttachWQFd(t *testing.T) {
	boss, bossErr := iouring.New(iouring.WithEntries(4), iouring.WithFlags(iouring.SetupSQPoll))
	if bossErr != nil {
		t.Error(bossErr)
		return
	}
	defer boss.Close()
	t.Log("boss:", boss.SQEntries(), boss.CQEntries())
	worker, workerErr := iouring.New(iouring.WithEntries(8), iouring.WithFlags(iouring.SetupSQPoll), iouring.WithAttachWQFd(uint32(boss.Fd())))
	if workerErr != nil {
		t.Error(workerErr)
		return
	}
	defer worker.Close()
	t.Log("worker:", worker.SQEntries(), worker.CQEntries(), worker.Flags()&iouring.SetupAttachWQ)

	sq := boss.GetSQE()
	if sq == nil {
		t.Error("SQE is nil")
		return
	}
	sq.PrepareNop()
	sq.SetData64(1)

	n, subErr := boss.Submit()
	if subErr != nil {
		t.Error(subErr)
		return
	}
	t.Log("sub:", n)

	ts := syscall.NsecToTimespec(time.Second.Nanoseconds())
	cqe, waitErr := boss.WaitCQETimeout(&ts)
	if waitErr != nil {
		t.Error(waitErr)
		return
	}
	if cqe.UserData == sq.UserData {
		t.Log("succeed")
	} else {
		t.Error("UserData not equal")
	}

	sq = worker.GetSQE()
	if sq == nil {
		t.Error("SQE is nil")
		return
	}
	sq.PrepareNop()
	sq.SetData64(1)

	n, subErr = worker.Submit()
	if subErr != nil {
		t.Error(subErr)
		return
	}
	t.Log("sub:", n)

	ts = syscall.NsecToTimespec(time.Second.Nanoseconds())
	cqe, waitErr = worker.WaitCQETimeout(&ts)
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
