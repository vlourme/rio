//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func Acquire() (v *Vortex, err error) {
	pollOnce.Do(func() {
		err = pollInit()
	})
	if err != nil {
		return
	}
	if n := pollAcquires.Add(1); n == 1 {
		if err == nil {
			err = poll.Start(context.Background())
		}
	}
	v = poll
	return
}

func Release(v *Vortex) (err error) {
	if v == nil {
		err = errors.New("release vortex is nil")
		return
	}
	if n := pollAcquires.Add(-1); n == 0 {
		err = poll.Shutdown()
	}
	return
}

func PrepareInitIOURingOptions(options ...Option) {
	pollOptions = append(pollOptions, options...)
}

var (
	pollOnce     sync.Once
	poll         *Vortex
	pollAcquires atomic.Int64
	pollOptions  []Option = make([]Option, 0, 1)
)

const (
	envEntries              = "IOURING_ENTRIES"
	envFlags                = "IOURING_SETUP_FLAGS"
	envFlagsSchema          = "IOURING_SETUP_FLAGS_SCHEMA"
	envSQThreadCPU          = "IOURING_SQ_THREAD_CPU"
	envSQThreadIdle         = "IOURING_SQ_THREAD_IDLE"
	envPrepSQEBatchSize     = "IOURING_PREP_SQE_BATCH_SIZE"
	envPrepSQEIdleTime      = "IOURING_PREP_SQE_IDLE_TIME"
	envWaitCQEBatchSize     = "IOURING_WAIT_CQE_BATCH_SIZE"
	envWaitCQETimeCurve     = "IOURING_WAIT_CQE_TIME_CURVE"
	envRegisterFixedBuffers = "IOURING_REG_FIXED_BUFFERS"
)

func pollInit() (err error) {
	if len(pollOptions) == 0 {
		entries := loadEnvEntries()
		pollOptions = append(pollOptions, WithEntries(int(entries)))

		flags := loadEnvFlags()
		pollOptions = append(pollOptions, WithFlags(flags))

		sqThreadCPU := loadEnvSQThreadCPU()
		pollOptions = append(pollOptions, WithSQThreadCPU(sqThreadCPU))

		sqThreadIdle := loadEnvSQThreadIdle()
		pollOptions = append(pollOptions, WithSQThreadIdle(sqThreadIdle))

		bufs, bufc := loadEnvRegFixedBuffers()
		pollOptions = append(pollOptions, WithRegisterFixedBuffer(bufs, bufc))

		prepareBatchSize := loadEnvPrepareSQBatchSize()
		pollOptions = append(pollOptions, WithPrepareSQBatchSize(prepareBatchSize))

		prepareIdleTime := loadEnvPrepareSQIdleTime()
		pollOptions = append(pollOptions, WithPrepareSQIdleTime(prepareIdleTime))

		waitCQBatchSize := loadEnvWaitCQBatchSize()
		pollOptions = append(pollOptions, WithWaitCQBatchSize(waitCQBatchSize))

		curve := loadEnvWaitCQTimeCurve()
		pollOptions = append(pollOptions, WithWaitCQTimeCurve(curve))

	}

	poll, err = New(pollOptions...)
	return
}

func loadEnvEntries() uint32 {
	s, has := os.LookupEnv(envEntries)
	if !has {
		return 0
	}
	u, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if parseErr != nil {
		return 0
	}
	return uint32(u)
}

func loadEnvFlags() uint32 {
	s, has := os.LookupEnv(envFlags)
	if !has {
		if schema, ok := os.LookupEnv(envFlagsSchema); ok {
			schema = strings.TrimSpace(schema)
			schema = strings.ToUpper(schema)
			switch schema {
			case DefaultFlagsSchema:
				return defaultIOURingSetupFlags()
			case PerformanceFlagsSchema:
				return performanceIOURingSetupFlags()
			default:
				return defaultIOURingSetupFlags()
			}
		}
		return defaultIOURingSetupFlags()
	}
	flags := uint32(0)
	ss := strings.Split(s, ",")
	for _, s0 := range ss {
		parsed := iouring.ParseSetupFlags(s0)
		if parsed > 0 {
			flags |= parsed
		}
	}
	return flags
}

func loadEnvSQThreadCPU() uint32 {
	s, has := os.LookupEnv(envSQThreadCPU)
	if !has {
		return 0
	}
	u, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if parseErr != nil {
		return 0
	}
	return uint32(u)
}

func loadEnvSQThreadIdle() uint32 {
	s, has := os.LookupEnv(envSQThreadIdle)
	if !has {
		return 0
	}
	u, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if parseErr != nil {
		return 0
	}
	return uint32(u)
}

func loadEnvPrepareSQBatchSize() uint32 {
	s, has := os.LookupEnv(envPrepSQEBatchSize)
	if !has {
		return 0
	}
	u, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if parseErr != nil {
		return 0
	}
	return uint32(u)
}

func loadEnvPrepareSQIdleTime() time.Duration {
	s, has := os.LookupEnv(envPrepSQEIdleTime)
	if !has {
		return 0
	}
	d, parseErr := time.ParseDuration(strings.TrimSpace(s))
	if parseErr != nil {
		return 0
	}
	return d
}

func loadEnvWaitCQBatchSize() uint32 {
	s, has := os.LookupEnv(envWaitCQEBatchSize)
	if !has {
		return 0
	}
	u, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if parseErr != nil {
		return 0
	}
	return uint32(u)
}

func loadEnvWaitCQTimeCurve() Curve {
	s, has := os.LookupEnv(envWaitCQETimeCurve)
	if !has {
		return nil
	}
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	ss := strings.Split(s, ",")
	curve := make(Curve, 0, 1)
	for _, s0 := range ss {
		i := strings.Index(s0, ":")
		if i == -1 {
			return nil
		}
		ns := strings.TrimSpace(s0[:i])
		n, nErr := strconv.ParseUint(ns, 10, 32)
		if nErr != nil {
			return nil
		}
		ts := strings.TrimSpace(s0[i+1:])
		t, tErr := time.ParseDuration(ts)
		if tErr != nil {
			return nil
		}
		curve = append(curve, struct {
			N       uint32
			Timeout time.Duration
		}{N: uint32(n), Timeout: t})
	}
	return curve
}

func loadEnvRegFixedBuffers() (size uint32, count uint32) {
	s, has := os.LookupEnv(envRegisterFixedBuffers)
	if !has {
		return
	}
	idx := strings.IndexByte(s, ',')
	if idx < 1 {
		return
	}
	ss := strings.TrimSpace(s[:idx])
	us, parseSizeErr := strconv.ParseUint(ss, 10, 32)
	if parseSizeErr != nil {
		return
	}

	cs := strings.TrimSpace(s[idx+1:])
	uc, parseCountErr := strconv.ParseUint(cs, 10, 32)
	if parseCountErr != nil {
		return
	}

	size = uint32(us)
	count = uint32(uc)
	return
}
