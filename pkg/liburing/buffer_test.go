//go:build linux

package liburing_test

import (
	"bytes"
	"crypto/rand"
	"github.com/brickingsoft/rio/pkg/liburing"
	"syscall"
	"testing"
	"unsafe"
)

func TestBufferAndRing_BufRingAdd(t *testing.T) {
	var fds [2]int
	if pipeErr := syscall.Pipe2(fds[:], syscall.O_CLOEXEC|syscall.O_NONBLOCK); pipeErr != nil {
		t.Error(pipeErr)
		return
	}
	var (
		rfd = fds[0]
		wfd = fds[1]
	)
	defer syscall.Close(rfd)
	defer syscall.Close(wfd)

	ring, ringErr := liburing.New(liburing.WithEntries(4))
	if ringErr != nil {
		t.Error(ringErr)
		return
	}
	defer ring.Close()

	var (
		entries uint16 = 4
	)
	buffers := int(entries)
	bufferLen := 4096
	b := make([]byte, bufferLen*buffers)

	config, configErr := liburing.NewBufferAndRingConfig(ring, 0, entries, 0, b)
	if configErr != nil {
		t.Error(configErr)
		return
	}

	var (
		sqe    *liburing.SubmissionQueueEntry
		sErr   error
		cqe    *liburing.CompletionQueueEvent
		cqeErr error
	)

	wp := make([]byte, bufferLen)
	_, _ = rand.Read(wp)

	// send and recv
	for i := 0; i < 6; i++ {
		sqe = ring.GetSQE()
		sqe.PrepareWrite(wfd, uintptr(unsafe.Pointer(&wp[0])), uint32(bufferLen), 0)
		_, sErr = ring.SubmitAndWait(1)
		if sErr != nil {
			t.Error(sErr)
			return
		}
		cqe, cqeErr = ring.PeekCQE()
		if cqeErr != nil {
			t.Error(cqeErr)
			return
		}
		if cqe.Res < 0 {
			t.Error("send:", syscall.Errno(-cqe.Res))
			return
		}
		ring.CQAdvance(1)

		sqe = ring.GetSQE()
		sqe.PrepareRead(rfd, 0, 0, 0)
		sqe.SetBufferGroup(0)
		sqe.Flags |= liburing.IOSQE_BUFFER_SELECT
		_, sErr = ring.SubmitAndWait(1)
		if sErr != nil {
			t.Error(sErr)
			return
		}
		cqe, cqeErr = ring.PeekCQE()
		if cqeErr != nil {
			t.Error(cqeErr)
			return
		}
		bid := config.Bid(cqe)
		rn := cqe.Res
		t.Log("recv:", cqe.Res, cqe.Flags, bid)
		if cqe.Res < 0 {
			t.Error("recv:", syscall.Errno(-cqe.Res))
			config.Advance(1)
			ring.CQAdvance(1)
			continue
		}

		config.Advance(1)
		ring.CQAdvance(1)
		rp := b[int(bid)*bufferLen : (int(bid)+1)*bufferLen][:rn]
		t.Log("recv ok:", bytes.Equal(rp, wp))
	}

	closeErr := config.Close()
	if closeErr != nil {
		t.Error(closeErr)
	}
}
