//go:build !amd64 && !arm64 && !ppc64 && !ppc64le && !riscv64 && !s390x

package liburing

import "math"

func RoundupPow2(n uint32) uint32 {
	if n < 1 {
		return 0
	}

	x := uint32(n - 1)
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16

	if x >= uint32(math.MaxInt32) {
		return uint32(math.MaxInt32)
	}

	return uint32(x + 1)
}
