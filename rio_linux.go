//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Presets
// preset aio options, must be called before Pin, Dial and Listen.
func Presets(options ...aio.Option) {
	vortexInstanceOptions = append(vortexInstanceOptions, options...)
}

var (
	vortexInstance        *aio.Vortex
	vortexInstanceOptions []aio.Option
	vortexInstanceErr     error
	vortexInstanceOnce    sync.Once
)

const (
	envEntries                    = "RIO_IOURING_ENTRIES"
	envFlags                      = "RIO_IOURING_SETUP_FLAGS"
	envSQThreadCPU                = "RIO_IOURING_SQ_THREAD_CPU"
	envSQThreadIdle               = "RIO_IOURING_SQ_THREAD_IDLE"
	envRegisterFixedBuffers       = "RIO_IOURING_REG_FIXED_BUFFERS"
	envRegisterFixedFiles         = "RIO_IOURING_REG_FIXED_FILES"
	envRegisterReservedFixedFiles = "RIO_IOURING_REG_FIXED_FILES_RESERVED"
	envPrepSQEAffCPU              = "RIO_PREP_SQE_AFF_CPU"
	envPrepSQEBatchMinSize        = "RIO_PREP_SQE_BATCH_MIN_SIZE"
	envPrepSQEBatchTimeWindow     = "RIO_PREP_SQE_BATCH_TIME_WINDOW"
	envPrepSQEBatchIdleTime       = "RIO_PREP_SQE_BATCH_IDLE_TIME"
	envWaitCQEMode                = "RIO_WAIT_CQE_MODE"
	envWaitCQETimeCurve           = "RIO_WAIT_CQE_TIME_CURVE"
	envWaitCQEPullIdleTime        = "RIO_WAIT_CQE_PULL_IDLE_TIME"
)

func getVortex() (*aio.Vortex, error) {
	vortexInstanceOnce.Do(func() {
		if len(vortexInstanceOptions) == 0 { // use env
			vortexInstanceOptions = make([]aio.Option, 0, 1)

			// ring >>>
			if v, has := envLoadUint32(envEntries); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithEntries(v))
			}

			if v, has := envLoadFlags(envFlags); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithFlags(v))
			} else {
				if cpus := runtime.NumCPU(); cpus > 3 { // use sq_poll
					v = iouring.SetupSQPoll | iouring.SetupSQAff | iouring.SetupSingleIssuer
				} else { // use coop task run
					v = iouring.SetupCoopTaskRun | iouring.SetupDeferTaskRun
				}
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithFlags(v))
			}

			if v, has := envLoadUint32(envSQThreadCPU); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithSQThreadCPU(v))
			}
			if v, has := envLoadDuration(envSQThreadIdle); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithSQThreadIdle(v))
			}
			// ring <<<

			// fixed >>>
			if v0, v1, has := envLoadUint32Coop(envRegisterFixedBuffers); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithRegisterFixedBuffer(v0, v1))
			}
			if v, has := envLoadUint32(envRegisterFixedFiles); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithRegisterFixedFiles(v))
			}
			if v, has := envLoadUint32(envRegisterReservedFixedFiles); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithRegisterReservedFixedFiles(v))
			}
			// fixed <<<

			// prep >>>
			if v, has := envLoadUint32(envPrepSQEAffCPU); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithPrepSQEAFFCPU(int(v)))
			}
			if v, has := envLoadUint32(envPrepSQEBatchMinSize); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithPrepSQEBatchMinSize(v))
			}
			if v, has := envLoadDuration(envPrepSQEBatchTimeWindow); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithPrepSQEBatchTimeWindow(v))
			}
			if v, has := envLoadDuration(envPrepSQEBatchIdleTime); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithPrepSQEBatchIdleTime(v))
			}
			// prep <<<

			// wait >>>
			if v, has := envLoadString(envWaitCQEMode); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithWaitCQEMode(v))
			}
			if v, has := envLoadCurve(envWaitCQETimeCurve); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithWaitCQETimeCurve(v))
			}
			if v, has := envLoadDuration(envWaitCQEPullIdleTime); has {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithWaitCQEPullIdleTime(v))
			}

			// wait <<<

		}
		// open
		vortexInstance, vortexInstanceErr = aio.Open(context.Background(), vortexInstanceOptions...)
	})
	return vortexInstance, vortexInstanceErr
}

func envLoadString(name string) (string, bool) {
	s, has := os.LookupEnv(name)
	if !has {
		return "", false
	}
	return strings.TrimSpace(s), true
}

func envLoadUint32(name string) (uint32, bool) {
	s, has := os.LookupEnv(name)
	if !has {
		return 0, false
	}
	u, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if parseErr != nil {
		return 0, false
	}
	return uint32(u), true
}

func envLoadUint32Coop(name string) (uint32, uint32, bool) {
	s, has := os.LookupEnv(name)
	if !has {
		return 0, 0, false
	}
	idx := strings.IndexByte(s, ',')
	if idx < 1 {
		return 0, 0, false
	}
	ss := strings.TrimSpace(s[:idx])
	u0, parseSizeErr := strconv.ParseUint(ss, 10, 32)
	if parseSizeErr != nil {
		return 0, 0, false
	}

	cs := strings.TrimSpace(s[idx+1:])
	u1, parseCountErr := strconv.ParseUint(cs, 10, 32)
	if parseCountErr != nil {
		return 0, 0, false
	}
	return uint32(u0), uint32(u1), true
}

func envLoadFlags(name string) (uint32, bool) {
	s, has := os.LookupEnv(name)
	if !has {
		return 0, false
	}
	flags := uint32(0)
	ss := strings.Split(s, ",")
	for _, s0 := range ss {
		parsed := iouring.ParseSetupFlags(s0)
		if parsed > 0 {
			flags |= parsed
		}
	}
	return flags, true
}

func envLoadDuration(name string) (time.Duration, bool) {
	s, has := os.LookupEnv(name)
	if !has {
		return 0, false
	}
	d, parseErr := time.ParseDuration(strings.TrimSpace(s))
	if parseErr != nil {
		return 0, false
	}
	return d, true
}

func envLoadCurve(name string) (aio.Curve, bool) {
	s, has := os.LookupEnv(name)
	if !has {
		return nil, false
	}
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	ss := strings.Split(s, ",")
	curve := make(aio.Curve, 0, 1)
	for _, s0 := range ss {
		i := strings.Index(s0, ":")
		if i == -1 {
			return nil, false
		}
		ns := strings.TrimSpace(s0[:i])
		n, nErr := strconv.ParseUint(ns, 10, 32)
		if nErr != nil {
			return nil, false
		}
		ts := strings.TrimSpace(s0[i+1:])
		t, tErr := time.ParseDuration(ts)
		if tErr != nil {
			return nil, false
		}
		curve = append(curve, struct {
			N       uint32
			Timeout time.Duration
		}{N: uint32(n), Timeout: t})
	}
	return curve, true
}
