package liburing

import "math/bits"

func RoundupPow2(n uint32) uint32 {
	if n <= 1 {
		return 1
	}
	shift := bits.Len(uint(n) - 1)
	return 1 << shift
}

func FloorPow2(n uint32) uint32 {
	if n == 0 {
		return 0
	}
	shift := bits.Len(uint(n)) - 1
	return 1 << shift
}
