package rio

import "github.com/brickingsoft/rio/pkg/liburing/aio"

// Preset aio options
func Preset(options ...aio.Option) {
	aio.Preset(options...)
}
