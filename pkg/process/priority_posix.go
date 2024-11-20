//go:build !windows && !unix

package process

func SetCurrentProcessPriority(level PriorityLevel) (err error) {

	return
}
