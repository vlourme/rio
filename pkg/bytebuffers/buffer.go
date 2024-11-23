package bytebuffers

import (
	"errors"
	"os"
)

type Buffer interface {
	Len() (n int)
	Cap() (n int)
	Peek(n int) (p []byte)
	Next(n int) (p []byte, err error)
	Discard(n int) (err error)
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Allocate(size int) (p []byte, err error)
	AllocatedWrote(n int) (err error)
	WritePending() bool
	Reset()
}

var (
	pageszie               = os.Getpagesize()
	oneQuarterOfPagesize   = pageszie / 4
	halfOfPagesize         = pageszie / 2
	threeQuarterOfPagesize = oneQuarterOfPagesize * 3
)

var (
	ErrTooLarge                  = errors.New("bytebuffers.Buffer: too large")
	ErrWriteBeforeAllocatedWrote = errors.New("bytebuffers: cannot write before AllocatedWrote(), cause prev Allocate() was not finished, please call AllocatedWrote() after the area was wrote")
	ErrAllocateZero              = errors.New("bytebuffers: cannot allocate zero")
)

var errNegativeRead = errors.New("bytebuffers.Buffer: reader returned negative count from Read")
