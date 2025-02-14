package ring

import "github.com/brickingsoft/errors"

var (
	ErrUncompleted = errors.Define("uncompleted")
	ErrTimeout     = errors.Define("timeout")
)

func IsUncompleted(err error) bool {
	return errors.Is(err, ErrUncompleted)
}

func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout)
}
