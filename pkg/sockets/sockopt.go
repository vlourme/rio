//go:build unix || windows

package sockets

func boolint(b bool) int {
	if b {
		return 1
	}
	return 0
}
