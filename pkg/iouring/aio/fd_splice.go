//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio/sys"
	"golang.org/x/sys/unix"
)

const (
	maxSpliceSize = 1 << 20
)

func (fd *Fd) Splice(src int, remain int64) (n int64, err error) {
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
		drainParams := SpliceParams{
			FdIn:   src,
			OffIn:  -1,
			FdOut:  pipe.WriterFd(),
			OffOut: -1,
			NBytes: uint32(chunk),
			Flags:  unix.SPLICE_F_NONBLOCK,
		}
		opDrain := fd.vortex.acquireOperation()
		opDrain.PrepareSplice(&drainParams)
		if ok := fd.vortex.Submit(opDrain); !ok {
			err = ErrCanceled
			return
		}
		drained, _, drainedErr := fd.vortex.awaitOperation(opDrain)
		fd.vortex.releaseOperation(opDrain)
		if drainedErr != nil || drained == 0 {
			err = drainedErr
			break
		}
		pipe.DrainN(drained)
		// pump
		pumpParams := SpliceParams{
			FdIn:   pipe.ReaderFd(),
			OffIn:  -1,
			FdOut:  fd.regular,
			OffOut: -1,
			NBytes: uint32(drained),
			Flags:  unix.SPLICE_F_NONBLOCK,
		}
		opPump := fd.vortex.acquireOperation()
		opPump.PrepareSplice(&pumpParams)
		if ok := fd.vortex.Submit(opPump); !ok {
			err = ErrCanceled
			return
		}
		pumped, _, pumpedErr := fd.vortex.awaitOperation(opPump)
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
