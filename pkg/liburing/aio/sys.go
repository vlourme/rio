//go:build linux

package aio

func boolint(b bool) int {
	if b {
		return 1
	}
	return 0
}
