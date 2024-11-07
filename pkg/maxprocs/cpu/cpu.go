package cpu

import "math"

type QuotaStatus int

const (
	QuotaUndefined QuotaStatus = iota
	QuotaUsed
	QuotaMinUsed
)

func DefaultRoundFunc(v float64) int {
	return int(math.Floor(v))
}
