//go:build !amd64 && !arm64 && !ppc64 && !ppc64le && !riscv64 && !s390x

package aio

import "math"

func RoundupPow2(n int) int {
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
		return math.MaxInt32
	}

	return int(x + 1)
}
