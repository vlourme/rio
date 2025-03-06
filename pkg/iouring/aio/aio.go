package aio

import (
	"context"
	"errors"
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
	if n := pollInited.Add(1); n == 1 {
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
	if n := pollInited.Add(-1); n == 0 {
		err = poll.Shutdown()
	}
	return
}

var (
	pollOnce   sync.Once
	poll       *Vortex
	pollInited atomic.Int64
)

const (
	envEntries           = "IOURING_ENTRIES"
	envFlags             = "IOURING_FLAGS"
	envFeatures          = "IOURING_FEATURES"
	envPrepareBatchSize  = "IOURING_PREPARE_BATCH_SIZE"
	envUseCPUAffinity    = "IOURING_USE_CPU_AFFILIATE"
	envCurveTransmission = "IOURING_CURVE_TRANSMISSION"
)

func pollInit() (err error) {
	opts := make([]Option, 0, 1)
	entries := loadEnvEntries()
	opts = append(opts, WithEntries(int(entries)))
	flags := loadEnvFlags()
	opts = append(opts, WithFlags(flags))
	features := loadEnvFeatures()
	opts = append(opts, WithFeatures(features))
	prepareBatchSize := loadEnvPrepareBatchSize()
	opts = append(opts, WithPrepareBatchSize(prepareBatchSize))
	useCPUAffinity := loadEnvUseCPUAffinity()
	opts = append(opts, WithUseCPUAffinity(useCPUAffinity))
	curveTransmission := loadEnvCurveTransmission()
	if len(curveTransmission) > 0 {
		opts = append(opts, WithWaitTransmission(NewCurveTransmission(curveTransmission)))
	}
	poll, err = New()
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
		return 0
	}
	flags := uint32(0)
	ss := strings.Split(s, "|")
	for _, s0 := range ss {
		u, parseErr := strconv.ParseUint(strings.TrimSpace(s0), 10, 32)
		if parseErr != nil {
			return 0
		}
		flags |= uint32(u)
	}
	return flags
}

func loadEnvFeatures() uint32 {
	s, has := os.LookupEnv(envFeatures)
	if !has {
		return 0
	}
	feats := uint32(0)
	ss := strings.Split(s, "|")
	for _, s0 := range ss {
		u, parseErr := strconv.ParseUint(strings.TrimSpace(s0), 10, 32)
		if parseErr != nil {
			return 0
		}
		feats |= uint32(u)
	}
	return feats
}

func loadEnvPrepareBatchSize() uint32 {
	s, has := os.LookupEnv(envPrepareBatchSize)
	if !has {
		return 0
	}
	u, parseErr := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if parseErr != nil {
		return 0
	}
	return uint32(u)
}

func loadEnvUseCPUAffinity() bool {
	s, has := os.LookupEnv(envUseCPUAffinity)
	if !has {
		return false
	}
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	return s == "true" || s == "1"
}

func loadEnvCurveTransmission() Curve {
	s, has := os.LookupEnv(envCurveTransmission)
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
