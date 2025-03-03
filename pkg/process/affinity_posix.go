//go:build !linux

package process

func SetCPUAffinity(index int) error {
	return nil
}
