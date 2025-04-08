//go:build linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio"
	"github.com/brickingsoft/rio/pkg/reference"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func Pin() {
	rc, rcErr := getAsyncIO()
	if rcErr != nil {
		panic(rcErr)
		return
	}
	rc.Pin()
}

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

const (
	envEntries               = "RIO_IOURING_ENTRIES"
	envFlags                 = "RIO_IOURING_SETUP_FLAGS"
	envSQThreadIdle          = "RIO_IOURING_SQ_THREAD_IDLE"
	envSendZCEnabled         = "RIO_IOURING_SENDZC_ENABLED"
	envMultishotDisabled     = "RIO_IOURING_MULTISHOT_DISABLED"
	envBufferAndBufferConfig = "RIO_IOURING_BUFFER_AND_RING"   // 4096x16, 15s
	envWaitCQETimeCurve      = "RIO_IOURING_WAIT_TIME_CURVE"   // 2:1us, 8:10us
	envWaitCQEIdleTimeout    = "RIO_IOURING_WAIT_IDLE_TIMEOUT" // 2:1us, 8:10us
)

func getAsyncIO() (*reference.Pointer[aio.AsyncIO], error) {
	aioInstanceOnce.Do(func() {
		if len(aioOptions) == 0 { // use env
			aioOptions = make([]aio.Option, 0, 1)

			// ring >>>
			if v, has := envLoadUint32(envEntries); has {
				aioOptions = append(aioOptions, aio.WithEntries(v))
			}

			if v, has := envLoadFlags(envFlags); has {
				aioOptions = append(aioOptions, aio.WithFlags(v))
			} else {
				v = liburing.IORING_SETUP_COOP_TASKRUN |
					liburing.IORING_SETUP_TASKRUN_FLAG |
					liburing.IORING_SETUP_SINGLE_ISSUER |
					liburing.IORING_SETUP_DEFER_TASKRUN
				aioOptions = append(aioOptions, aio.WithFlags(v))
			}

			if v, has := envLoadDuration(envSQThreadIdle); has {
				aioOptions = append(aioOptions, aio.WithSQThreadIdle(v))
			}
			if ok := envLoadBool(envSendZCEnabled); ok {
				aioOptions = append(aioOptions, aio.WithSendZCEnabled(ok))
			}
			if ok := envLoadBool(envMultishotDisabled); ok {
				aioOptions = append(aioOptions, aio.WithMultiShotDisabled(ok))
			}
			// ring <<<

			// buffer >>>
			if size, count, idleTimeout, has := envLoadBufferAndRingConfig(envBufferAndBufferConfig); has {
				aioOptions = append(aioOptions, aio.WithRingBufferConfig(size, count, idleTimeout))
			}
			// buffer <<<

			// cqe >>>
			if v, has := envLoadDuration(envWaitCQEIdleTimeout); has {
				aioOptions = append(aioOptions, aio.WithWaitCQEIdleTimeout(v))
			}
			if v, has := envLoadCurve(envWaitCQETimeCurve); has {
				aioOptions = append(aioOptions, aio.WithWaitCQETimeCurve(v))
			}
			// cqe <<<

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

func envLoadBool(name string) bool {
	s, has := os.LookupEnv(name)
	if !has {
		return false
	}
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "true", "1", "y", "yes":
		return true
	default:
		return false
	}
}

func envLoadFlags(name string) (uint32, bool) {
	s, has := os.LookupEnv(name)
	if !has {
		return 0, false
	}
	flags := uint32(0)
	ss := strings.Split(s, ",")
	for _, s0 := range ss {
		parsed := liburing.ParseSetupFlags(s0)
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

func envLoadBufferAndRingConfig(name string) (size int, count int, idleTimeout time.Duration, has bool) {
	s, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	// {size}x{count}, 1ms
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	var (
		s1 string
		s2 string
	)

	idx := strings.Index(s, ",")
	if idx == -1 {
		s1 = s
	} else {
		s1 = strings.TrimSpace(s[:idx])
		s2 = strings.TrimSpace(s[idx+1:])
	}

	if s1 != "" {
		ss := strings.Split(s1, "x")
		if len(ss) != 2 {
			return
		}
		var err error
		size, err = strconv.Atoi(ss[0])
		if err != nil {
			return
		}
		count, err = strconv.Atoi(ss[1])
		if err != nil {
			return
		}
	}

	if s2 != "" {
		var idleTimeoutErr error
		idleTimeout, idleTimeoutErr = time.ParseDuration(s2)
		if idleTimeoutErr != nil {
			return
		}
	}

	has = s1 != "" || s2 != ""
	return
}
