package aio_test

import (
	"testing"
)

func roundupPow2(n int) int {
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

func TestPow(t *testing.T) {
	for i := 0; i < 10; i++ {
		t.Log(i, roundupPow2(i+1))
	}
}
