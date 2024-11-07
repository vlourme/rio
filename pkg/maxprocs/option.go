package maxprocs

import "github.com/brickingsoft/rio/pkg/maxprocs/cpu"

type Options struct {
	procs          func(int, func(v float64) int) (int, cpu.QuotaStatus, error)
	minGOMAXPROCS  int
	roundQuotaFunc func(v float64) int
}

type Option func(*Options) error

func Min(n int) Option {
	return func(o *Options) error {
		if n > 2 {
			o.minGOMAXPROCS = n
		}
		return nil
	}
}

func RoundQuotaFunc(rf func(v float64) int) Option {
	return func(o *Options) error {
		o.roundQuotaFunc = rf
		return nil
	}
}
