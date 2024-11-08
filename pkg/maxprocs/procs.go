package maxprocs

import (
	"github.com/brickingsoft/rio/pkg/maxprocs/cpu"
	"os"
	"runtime"
)

const maxProcsEnvKey = "GOMAXPROCS"

func currentMaxProcs() int {
	return runtime.GOMAXPROCS(0)
}

type Undo func()

func Enable(opts ...Option) (undo Undo, err error) {
	options := &Options{
		procs:          cpu.QuotaToGOMAXPROCS,
		roundQuotaFunc: cpu.DefaultRoundFunc,
		minGOMAXPROCS:  1,
	}
	for _, opt := range opts {
		err = opt(options)
		if err != nil {
			return nil, err
		}
	}

	undo = func() {}

	if _, exists := os.LookupEnv(maxProcsEnvKey); exists {
		return
	}

	maxProcs, status, procsErr := options.procs(options.minGOMAXPROCS, options.roundQuotaFunc)
	if procsErr != nil {
		err = procsErr
		return
	}

	if status == cpu.QuotaUndefined {
		return
	}

	prev := currentMaxProcs()
	undo = func() {
		runtime.GOMAXPROCS(prev)
	}

	runtime.GOMAXPROCS(maxProcs)
	return
}
