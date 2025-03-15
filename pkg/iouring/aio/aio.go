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
	pollOptions  = make([]Option, 0, 1)
)

const (
	envEntries                = "RIO_IOURING_ENTRIES"
	envFlags                  = "RIO_IOURING_SETUP_FLAGS"
	envFlagsSchema            = "RIO_IOURING_SETUP_FLAGS_SCHEMA"
	envSQThreadCPU            = "RIO_IOURING_SQ_THREAD_CPU"
	envSQThreadIdle           = "RIO_IOURING_SQ_THREAD_IDLE"
	envPrepSQEBatchSize       = "RIO_PREP_SQE_BATCH_SIZE"
	envPrepSQEBatchTimeWindow = "RIO_PREP_SQE_BATCH_TIME_WINDOW"
	envPrepSQEBatchIdleTime   = "RIO_PREP_SQE_BATCH_IDLE_TIME"
	envPrepSQEBatchAffCPU     = "RIO_PREP_SQE_BATCH_AFF_CPU"
	envWaitCQEBatchSize       = "RIO_WAIT_CQE_BATCH_SIZE"
	envWaitCQEBatchTimeCurve  = "RIO_WAIT_CQE_BATCH_TIME_CURVE"
	envWaitCQEBatchAffCPU     = "RIO_WAIT_CQE_BATCH_AFF_CPU"
	envRegisterFixedBuffers   = "RIO_REG_FIXED_BUFFERS"
)

func pollInit() (err error) {
	if len(pollOptions) == 0 {
		entries := loadEnvEntries()
		pollOptions = append(pollOptions, WithEntries(entries))

		flags := loadEnvFlags()
		pollOptions = append(pollOptions, WithFlags(flags))

		sqThreadCPU := loadEnvSQThreadCPU()
		pollOptions = append(pollOptions, WithSQThreadCPU(sqThreadCPU))

		sqThreadIdle := loadEnvSQThreadIdle()
		pollOptions = append(pollOptions, WithSQThreadIdle(sqThreadIdle))

		prepareBatchSize := loadEnvPrepareSQEBatchSize()
		pollOptions = append(pollOptions, WithPrepareSQEBatchSize(prepareBatchSize))

		prepareBatchTimeWindow := loadEnvPrepareSQETimeWindow()
		pollOptions = append(pollOptions, WithPrepSQEBatchTimeWindow(prepareBatchTimeWindow))

		prepareIdleTime := loadEnvPrepareSQEIdleTime()
		pollOptions = append(pollOptions, WithPrepareSQEBatchIdleTime(prepareIdleTime))

		prepareSQEAffCPU := loadEnvPrepareSQEAFFCPU()
		pollOptions = append(pollOptions, WithPrepareSQEBatchAFFCPU(prepareSQEAffCPU))

		waitCQBatchSize := loadEnvWaitCQEBatchSize()
		pollOptions = append(pollOptions, WithWaitCQEBatchSize(waitCQBatchSize))

		curve := loadEnvWaitCQETimeCurve()
		pollOptions = append(pollOptions, WithWaitCQEBatchTimeCurve(curve))

		waitCQEAffCPU := loadEnvWaitCQEAFFCPU()
		pollOptions = append(pollOptions, WithWaitCQEBatchAFFCPU(waitCQEAffCPU))

		bufs, bufc := loadEnvRegFixedBuffers()
		pollOptions = append(pollOptions, WithRegisterFixedBuffer(bufs, bufc))
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

func loadEnvPrepareSQEBatchSize() uint32 {
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

func loadEnvPrepareSQETimeWindow() time.Duration {
	s, has := os.LookupEnv(envPrepSQEBatchTimeWindow)
	if !has {
		return 0
	}
	d, parseErr := time.ParseDuration(strings.TrimSpace(s))
	if parseErr != nil {
		return 0
	}
	return d
}

func loadEnvPrepareSQEIdleTime() time.Duration {
	s, has := os.LookupEnv(envPrepSQEBatchIdleTime)
	if !has {
		return 0
	}
	d, parseErr := time.ParseDuration(strings.TrimSpace(s))
	if parseErr != nil {
		return 0
	}
	return d
}

func loadEnvPrepareSQEAFFCPU() int {
	s, has := os.LookupEnv(envPrepSQEBatchAffCPU)
	if !has {
		return -1
	}
	n, parseErr := strconv.Atoi(strings.TrimSpace(s))
	if parseErr != nil {
		return -1
	}
	return n
}

func loadEnvWaitCQEBatchSize() uint32 {
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

func loadEnvWaitCQETimeCurve() Curve {
	s, has := os.LookupEnv(envWaitCQEBatchTimeCurve)
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

func loadEnvWaitCQEAFFCPU() int {
	s, has := os.LookupEnv(envWaitCQEBatchAffCPU)
	if !has {
		return -1
	}
	n, parseErr := strconv.Atoi(strings.TrimSpace(s))
	if parseErr != nil {
		return -1
	}
	return n
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
