//go:build !linux

package aio

type Vortex struct{}

func (vortex *Vortex) Fd() int {
	return -1
}

func (vortex *Vortex) Close() (err error) {
	return
}
