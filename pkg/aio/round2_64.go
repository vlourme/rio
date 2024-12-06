//go:build amd64 || arm64 || ppc64 || ppc64le || riscv64 || s390x

package aio

func RoundupPow2(n int) int {
	if n < 1 {
		return 0
	}
	x := uint64(n - 1)
	x |= x >> 1
	x |= x >> 2
	x |= x >> 4
	x |= x >> 8
	x |= x >> 16
	return int(x + 1)
}
