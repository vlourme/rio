//go:build linux

package aio

func (fd *Fd) Poll(mask uint32) (n int, err error) {
	pfd := fd.direct
	fixed := true
	if pfd == -1 {
		fixed = false
		pfd = fd.regular
	}
	op := AcquireOperationWithDeadline(fd.readDeadline)
	op.PrepPollAdd(pfd, fixed, mask)
	n, _, err = poller.SubmitAndWait(op)
	ReleaseOperation(op)
	return
}
