package iouring

import (
	"math/bits"
	"unsafe"
)

func fls(x int) int {
	if x == 0 {
		return 0
	}

	return 8*int(unsafe.Sizeof(x)) - bits.LeadingZeros32(uint32(x))
}

func roundupPow2(depth uint32) uint32 {
	return 1 << uint32(fls(int(depth-1)))
}
