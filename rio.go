package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio"
	"github.com/brickingsoft/rio/pkg/reference"
)

var (
	aioInstance *reference.Pointer[aio.AsyncIO]
	aioOptions  []aio.Option
)

func Preset(options ...aio.Option) {
	aioOptions = append(aioOptions, options...)
}
