package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio"
	"github.com/brickingsoft/rio/pkg/reference"
)

var (
	vortexInstance        *reference.Pointer[*aio.Vortex]
	vortexInstanceOptions []aio.Option
)

func Pin() {
	if vortexInstance == nil {
		return
	}
	vortexInstance.Pin()
}

func Unpin() {
	if vortexInstance == nil {
		return
	}
	vortexInstance.Unpin()
}

func Preset(options ...aio.Option) {
	vortexInstanceOptions = append(vortexInstanceOptions, options...)
}
