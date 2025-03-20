//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio/sys"
	"golang.org/x/sys/unix"
)

const (
	maxSpliceSize = 1 << 20
)

func (fd *NetFd) Splice(src int, remain int64) (n int64, err error) {
	pipe, pipeErr := sys.AcquirePipe()
	if pipeErr != nil {
		return 0, pipeErr
	}
	defer sys.ReleasePipe(pipe)

	for err == nil && remain > 0 {
		chunk := int64(maxSpliceSize)
		if chunk > remain {
			chunk = remain
		}
		// drain
		opDrain := fd.vortex.acquireOperation()
		opDrain.PrepareSplice(src, -1, pipe.WriterFd(), -1, uint32(chunk), unix.SPLICE_F_NONBLOCK)
		fd.vortex.Submit(opDrain)
		drained, _, drainedErr := fd.vortex.awaitOperation(fd.ctx, opDrain)
		fd.vortex.releaseOperation(opDrain)
		if drainedErr != nil || drained == 0 {
			err = drainedErr
			break
		}
		pipe.DrainN(drained)
		// pump
		opPump := fd.vortex.acquireOperation()
		opPump.PrepareSplice(pipe.ReaderFd(), -1, fd.regular, -1, uint32(drained), unix.SPLICE_F_NONBLOCK)
		fd.vortex.Submit(opPump)
		pumped, _, pumpedErr := fd.vortex.awaitOperation(fd.ctx, opPump)
		fd.vortex.releaseOperation(opPump)
		if pumped > 0 {
			n += int64(pumped)
			remain -= int64(pumped)
			pipe.PumpN(pumped)
		}
		err = pumpedErr
	}
	return
}
