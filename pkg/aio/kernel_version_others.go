//go:build !linux

package aio

func KernelVersion() (major, minor int) {
	return 0, 0
}
