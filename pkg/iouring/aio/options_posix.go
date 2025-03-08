//go:build !linux

package aio

func defaultIOURingSetupFlags() uint32 {
	return 0
}

func performanceIOURingSetupFlags() uint32 {
	return 0
}
