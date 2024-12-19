//go:build dragonfly

package aio

func SetFastOpen(_ NetFd, _ bool) error {
	return nil
}
