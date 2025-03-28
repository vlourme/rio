package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio"
	"github.com/brickingsoft/rio/pkg/reference"
)

var (
	vortexInstance        *reference.Pointer[*aio.Vortex]
	vortexInstanceOptions []aio.Option
)

func Preset(options ...aio.Option) {
	vortexInstanceOptions = append(vortexInstanceOptions, options...)
}
