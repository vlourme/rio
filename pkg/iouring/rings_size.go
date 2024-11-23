package iouring

import "unsafe"

const ringSize = 320

func npages(size uint64, pageSize uint64) uint64 {
	size--
	size /= pageSize

	return uint64(fls(int(size)))
}

const (
	not63ul       = 18446744073709551552
	ringSizeCQOff = 63
)

// liburing: rings_size
func ringsSize(p *Params, entries uint32, cqEntries uint32, pageSize uint64) uint64 {
	var pages, sqSize, cqSize uint64

	cqSize = uint64(unsafe.Sizeof(CompletionQueueEvent{}))
	if p.flags&SetupCQE32 != 0 {
		cqSize += uint64(unsafe.Sizeof(CompletionQueueEvent{}))
	}
	cqSize *= uint64(cqEntries)
	cqSize += ringSize
	cqSize = (cqSize + ringSizeCQOff) & not63ul
	pages = 1 << npages(cqSize, pageSize)

	sqSize = uint64(unsafe.Sizeof(SubmissionQueueEntry{}))
	if p.flags&SetupSQE128 != 0 {
		sqSize += 64
	}
	sqSize *= uint64(entries)
	pages += 1 << npages(sqSize, pageSize)

	return pages * pageSize
}
