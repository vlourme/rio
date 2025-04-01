//go:build linux

package rio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio"
	"github.com/brickingsoft/rio/pkg/reference"
	"os"
	"runtime"
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
	envEntries                            = "RIO_IOURING_ENTRIES"
	envFlags                              = "RIO_IOURING_SETUP_FLAGS"
	envSQThreadCPU                        = "RIO_IOURING_SQ_THREAD_CPU"
	envSQThreadIdle                       = "RIO_IOURING_SQ_THREAD_IDLE"
	envSendZC                             = "RIO_IOURING_SENDZC"
	envDisableIOURingDirectAllocBlackList = "RIO_IOURING_DISABLE_IOURING_DIRECT_ALLOC_BLACKLIST" // a, b, c
	envRegisterFixedFiles                 = "RIO_IOURING_REG_FIXED_FILES"
	envIOURingHeartbeatTimeout            = "RIO_IOURING_HEARTBEAT_TIMEOUT"
	envBufferAndBufferConfig              = "RIO_BUFFER_AND_RING_CONFIG" // 4096x16x512, 15s
	envProducerLockOSThread               = "RIO_PRODUCER_LOCK_OSTHREAD"
	envProducerBatchSize                  = "RIO_PRODUCER_BATCH_SIZE"
	envProducerBatchTimeWindow            = "RIO_PRODUCER_BATCH_TIME_WINDOW"
	envProducerBatchIdleTime              = "RIO_PRODUCER_BATCH_IDLE_TIME"
	envConsumeBatchTimeCurve              = "RIO_CONSUMER_BATCH_TIME_CURVE" // 1:15s, 2:1us, 8:10us
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
				if cpus := runtime.NumCPU(); cpus > 1 { // use sq_poll
					v = liburing.IORING_SETUP_SQPOLL | liburing.IORING_SETUP_SINGLE_ISSUER
					if cpus > 3 {
						v |= liburing.IORING_SETUP_SQ_AFF
					}
				} else { // use coop task run
					v = liburing.IORING_SETUP_COOP_TASKRUN | liburing.IORING_SETUP_TASKRUN_FLAG
				}
				aioOptions = append(aioOptions, aio.WithFlags(v))
			}

			if v, has := envLoadUint32(envSQThreadCPU); has {
				aioOptions = append(aioOptions, aio.WithSQThreadCPU(v))
			}
			if v, has := envLoadDuration(envSQThreadIdle); has {
				aioOptions = append(aioOptions, aio.WithSQThreadIdle(v))
			}
			if ok := envLoadBool(envSendZC); ok {
				aioOptions = append(aioOptions, aio.WithSendZC(ok))
			}
			if v, has := envLoadStrings(envDisableIOURingDirectAllocBlackList); has {
				aioOptions = append(aioOptions, aio.WithDisableDirectAllocFeatKernelFlavorBlackList(v))
			} else {
				v = []string{"microsoft-standard-WSL2"}
				aioOptions = append(aioOptions, aio.WithDisableDirectAllocFeatKernelFlavorBlackList(v))
			}
			// ring <<<

			// fixed >>>
			if v, has := envLoadUint32(envRegisterFixedFiles); has {
				aioOptions = append(aioOptions, aio.WithRegisterFixedFiles(v))
			}
			// fixed <<<

			// buffer >>>
			if size, count, ref, idleTimeout, has := envLoadBufferAndRingConfig(envBufferAndBufferConfig); has {
				aioOptions = append(aioOptions, aio.WithRingBufferConfig(size, count, ref, idleTimeout))
			}
			// buffer <<<

			// heartbeat
			if v, has := envLoadDuration(envIOURingHeartbeatTimeout); has {
				aioOptions = append(aioOptions, aio.WithHeartBeatTimeout(v))
			}

			// sqe >>>
			producerOSThreadLock := envLoadBool(envProducerLockOSThread)
			producerBatchSize, _ := envLoadUint32(envProducerBatchSize)
			producerBatchTimeWindow, _ := envLoadDuration(envProducerBatchTimeWindow)
			producerBatchIdleTime, _ := envLoadDuration(envProducerBatchIdleTime)
			aioOptions = append(aioOptions, aio.WithProducer(producerOSThreadLock, producerBatchSize, producerBatchTimeWindow, producerBatchIdleTime))
			// sqe <<<

			// cqe >>>
			if v, has := envLoadCurve(envConsumeBatchTimeCurve); has {
				aioOptions = append(aioOptions, aio.WithConsumer(v))
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

func envLoadStrings(name string) ([]string, bool) {
	s, has := os.LookupEnv(name)
	if !has {
		return nil, false
	}
	ss := strings.Split(s, ",")
	for i := range ss {
		ss[i] = strings.TrimSpace(ss[i])
	}
	return ss, len(ss) > 0
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

func envLoadBufferAndRingConfig(name string) (size uint16, count uint16, ref uint16, idleTimeout time.Duration, has bool) {
	s, ok := os.LookupEnv(name)
	if !ok {
		return
	}
	// {size}x{count}x{ref}, 1ms
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
		if len(ss) != 3 {
			return
		}
		size64, size64Err := strconv.ParseUint(strings.TrimSpace(ss[0]), 10, 16)
		if size64Err != nil {
			return
		}
		size = uint16(size64)
		count64, count64Err := strconv.ParseUint(strings.TrimSpace(ss[1]), 10, 16)
		if count64Err != nil {
			return
		}
		count = uint16(count64)
		ref64, ref64Err := strconv.ParseUint(strings.TrimSpace(ss[2]), 10, 16)
		if ref64Err != nil {
			return
		}
		ref = uint16(ref64)
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
