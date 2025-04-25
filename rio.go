package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio"
)

var (
	aioOptions []aio.Option
)

// Preset aio options
func Preset(options ...aio.Option) {
	aioOptions = append(aioOptions, options...)
}
