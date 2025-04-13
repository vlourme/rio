//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"golang.org/x/sys/unix"
)

const (
	maxSpliceSize = 1 << 20
)

func (fd *Fd) Splice(src int, srcFixed bool, remain int64) (n int64, err error) {
	pipe, pipeErr := sys.AcquirePipe()
	if pipeErr != nil {
		return 0, pipeErr
	}
	defer sys.ReleasePipe(pipe)

	dst := fd.FileDescriptor()

	for err == nil && remain > 0 {
		chunk := int64(maxSpliceSize)
		if chunk > remain {
			chunk = remain
		}
		// drain
		drainParams := SpliceParams{
			FdIn:       src,
			FdInFixed:  srcFixed,
			OffIn:      -1,
			FdOut:      pipe.WriterFd(),
			FdOutFixed: false,
			OffOut:     -1,
			NBytes:     uint32(chunk),
			Flags:      unix.SPLICE_F_NONBLOCK,
		}
		opDrain := AcquireOperation()
		opDrain.PrepareSplice(&drainParams)
		drained, _, drainedErr := fd.eventLoop.SubmitAndWait(opDrain)
		ReleaseOperation(opDrain)
		if drainedErr != nil || drained == 0 {
			err = drainedErr
			break
		}
		pipe.DrainN(drained)
		// pump
		pumpParams := SpliceParams{
			FdIn:       pipe.ReaderFd(),
			FdInFixed:  false,
			OffIn:      -1,
			FdOut:      dst,
			FdOutFixed: true,
			OffOut:     -1,
			NBytes:     uint32(drained),
			Flags:      unix.SPLICE_F_NONBLOCK,
		}
		opPump := AcquireOperation()
		opPump.PrepareSplice(&pumpParams)
		pumped, _, pumpedErr := fd.eventLoop.SubmitAndWait(opPump)
		ReleaseOperation(opPump)
		if pumped > 0 {
			n += int64(pumped)
			remain -= int64(pumped)
			pipe.PumpN(pumped)
		}
		err = pumpedErr
	}
	return
}
