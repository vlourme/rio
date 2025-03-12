//go:build !linux

package kernel

func Get() Version {
	return Version{}
}
