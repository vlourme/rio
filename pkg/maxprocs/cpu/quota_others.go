//go:build !linux

package cpu

func QuotaToGOMAXPROCS(_ int, _ func(v float64) int) (int, QuotaStatus, error) {
	return -1, QuotaUndefined, nil
}
