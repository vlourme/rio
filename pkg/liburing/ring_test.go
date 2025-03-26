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
