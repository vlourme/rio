//go:build linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing/aio"
	"github.com/brickingsoft/rio/pkg/reference"
	"sync"
)

// Pin aio engine, use for dial only or multi listen.
func Pin() {
	rc, rcErr := getAsyncIO()
	if rcErr != nil {
		panic(rcErr)
		return
	}
	rc.Pin()
}

// Unpin aio engine
func Unpin() {
	rc, rcErr := getAsyncIO()
	if rcErr != nil {
		panic(rcErr)
		return
	}
	rc.Unpin()
}

var (
	aioInstanceErr  error
	aioInstanceOnce sync.Once
)

func getAsyncIO() (*reference.Pointer[aio.AsyncIO], error) {
	aioInstanceOnce.Do(func() {
		if len(aioOptions) == 0 {
			aioOptions = make([]aio.Option, 0, 1)
		}
		// open
		asyncIO, openErr := aio.Open(aioOptions...)
		if openErr != nil {
			aioInstanceErr = openErr
			return
		}
		aioInstance = reference.Make(asyncIO)
	})
	return aioInstance, aioInstanceErr
}
