//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/process"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// UseProcessPriority
// set process priority
func UseProcessPriority(level process.PriorityLevel) {
	_ = process.SetCurrentProcessPriority(level)
}

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

func getVortex() (*aio.Vortex, error) {
	vortexInstanceOnce.Do(func() {
		if len(vortexInstanceOptions) == 0 { // use env
			vortexInstanceOptions = make([]aio.Option, 0, 1)

			entries := loadEnvEntries()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithEntries(entries))

			flags, hasFlags := loadEnvFlags()
			schema := loadEnvFlagsSchema()
			if schema == "" {
				if hasFlags {
					vortexInstanceOptions = append(vortexInstanceOptions, aio.WithFlags(flags))
				} else {
					vortexInstanceOptions = append(vortexInstanceOptions, aio.WithFlagsSchema(aio.DefaultFlagsSchema))
				}
			} else {
				vortexInstanceOptions = append(vortexInstanceOptions, aio.WithFlagsSchema(schema))
			}

			sqThreadCPU := loadEnvSQThreadCPU()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithSQThreadCPU(sqThreadCPU))

			sqThreadIdle := loadEnvSQThreadIdle()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithSQThreadIdle(time.Duration(sqThreadIdle)*time.Millisecond))

			prepareBatchSize := loadEnvPrepareSQEBatchSize()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithPrepSQEBatchSize(prepareBatchSize))

			prepareBatchTimeWindow := loadEnvPrepareSQETimeWindow()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithPrepSQEBatchTimeWindow(prepareBatchTimeWindow))

			prepareIdleTime := loadEnvPrepareSQEIdleTime()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithPrepSQEBatchIdleTime(prepareIdleTime))

			prepareSQEAffCPU := loadEnvPrepareSQEAFFCPU()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithPrepSQEBatchAFFCPU(prepareSQEAffCPU))

			waitCQBatchSize := loadEnvWaitCQEBatchSize()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithWaitCQEBatchSize(waitCQBatchSize))

			curve := loadEnvWaitCQETimeCurve()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithWaitCQEBatchTimeCurve(curve))

			waitCQEAffCPU := loadEnvWaitCQEAFFCPU()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithWaitCQEBatchAFFCPU(waitCQEAffCPU))

			bufs, bufc := loadEnvRegFixedBuffers()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithRegisterFixedBuffer(bufs, bufc))

			files := loadEnvRegFixedFiles()
			vortexInstanceOptions = append(vortexInstanceOptions, aio.WithRegisterFixedFiles(files))
		}
		// open
		vortexInstance, vortexInstanceErr = aio.Open(context.Background(), vortexInstanceOptions...)
	})
	return vortexInstance, vortexInstanceErr
}

const (
	envEntries                = "RIO_IOURING_ENTRIES"
	envFlags                  = "RIO_IOURING_SETUP_FLAGS"
	envFlagsSchema            = "RIO_IOURING_SETUP_FLAGS_SCHEMA"
	envSQThreadCPU            = "RIO_IOURING_SQ_THREAD_CPU"
	envSQThreadIdle           = "RIO_IOURING_SQ_THREAD_IDLE"
	envRegisterFixedBuffers   = "RIO_IOURING_REG_FIXED_BUFFERS"
	envRegisterFixedFiles     = "RIO_IOURING_REG_FIXED_FILES"
	envPrepSQEBatchSize       = "RIO_PREP_SQE_BATCH_SIZE"
	envPrepSQEBatchTimeWindow = "RIO_PREP_SQE_BATCH_TIME_WINDOW"
	envPrepSQEBatchIdleTime   = "RIO_PREP_SQE_BATCH_IDLE_TIME"
	envPrepSQEBatchAffCPU     = "RIO_PREP_SQE_BATCH_AFF_CPU"
	envWaitCQEBatchSize       = "RIO_WAIT_CQE_BATCH_SIZE"
	envWaitCQEBatchTimeCurve  = "RIO_WAIT_CQE_BATCH_TIME_CURVE"
	envWaitCQEBatchAffCPU     = "RIO_WAIT_CQE_BATCH_AFF_CPU"
)

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

func loadEnvFlags() (uint32, bool) {
	s, has := os.LookupEnv(envFlags)
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

func loadEnvFlagsSchema() string {
	s, has := os.LookupEnv(envFlagsSchema)
	if !has {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(s))
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

func loadEnvWaitCQETimeCurve() aio.Curve {
	s, has := os.LookupEnv(envWaitCQEBatchTimeCurve)
	if !has {
		return nil
	}
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	ss := strings.Split(s, ",")
	curve := make(aio.Curve, 0, 1)
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

func loadEnvRegFixedFiles() uint32 {
	s, has := os.LookupEnv(envRegisterFixedFiles)
	if !has {
		return 0
	}
	u, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if parseErr != nil {
		return 0
	}
	return uint32(u)
}
