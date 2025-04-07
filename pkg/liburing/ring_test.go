//go:build linux

package liburing_test

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"testing"
)

func TestNew(t *testing.T) {
	ring, ringErr := liburing.New(liburing.WithEntries(4))
	if ringErr != nil {
		t.Error(ringErr)
		return
	}
	defer ring.Close()

	reg, regErr := ring.RegisterRingFd()
	if regErr != nil {
		t.Error(regErr)
		return
	}
	defer ring.UnregisterRingFd()
	t.Log("register ring fd success:", reg)

	t.Log("sq:", ring.SQEntries())
	t.Log("cq:", ring.CQEntries())
	probe, probeErr := ring.Probe()
	if probeErr != nil {
		t.Error(probeErr)
		return
	}
	t.Log("bind:", probe.IsSupported(liburing.IORING_OP_BIND))
	t.Log("listen:", probe.IsSupported(liburing.IORING_OP_LISTEN))
	t.Log("recv_zc:", probe.IsSupported(liburing.IORING_OP_RECV_ZC))

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

func TestRing_MSGRing(t *testing.T) {
	r1, r1Err := liburing.New(liburing.WithEntries(4))
	if r1Err != nil {
		t.Error(r1Err)
		return
	}
	defer r1.Close()

	_, reg1Err := r1.RegisterRingFd()
	if reg1Err != nil {
		t.Error(reg1Err)
		return
	}
	t.Log("register ring fd success:", r1.Fd(), r1.EnterFd())

	r2, r2Err := liburing.New(liburing.WithEntries(4))
	if r2Err != nil {
		t.Error(r2Err)
		return
	}
	defer r2.Close()

	_, reg2Err := r2.RegisterRingFd()
	if reg2Err != nil {
		t.Error(reg2Err)
		return
	}
	t.Log("register ring fd success:", r2.Fd(), r2.EnterFd())

	sqe := r1.GetSQE()
	sqe.PrepareMsgRing(r2.Fd(), 1, nil, 0)
	_, swErr := r1.SubmitAndWait(1)
	if swErr != nil {
		t.Error(swErr)
		return
	}

	cqe, cqeErr := r2.WaitCQE()
	if cqeErr != nil {
		t.Error(cqeErr)
		return
	}
	t.Log("cqe:", cqe.Res)

}
