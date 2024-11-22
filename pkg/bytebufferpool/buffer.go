package bytebufferpool

import "os"

type Buffer interface {
	Len() (n int)
	Cap() (n int)
	Available() (n int)
	Peek(n int) (p []byte)
	Next(n int) (p []byte, err error)
	Discard(n int)
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	WriteString(s string) (n int, err error)
	WriteByte(c byte) (err error)
	WriteRune(r rune) (n int, err error)
	Allocate(size int) (p []byte)
	AllocatedWrote(n int)
	WritePending() bool
	Empty() bool
	Reset()
}

var (
	pageszie               = os.Getpagesize()
	oneQuarterOfPagesize   = pageszie / 4
	halfOfPagesize         = pageszie / 2
	threeQuarterOfPagesize = oneQuarterOfPagesize * 3
)
