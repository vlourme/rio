//go:build linux

package cpu

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/maxprocs/cgroups"
)

func QuotaToGOMAXPROCS(minValue int, round func(v float64) int) (maxProcs int, status QuotaStatus, err error) {
	if round == nil {
		round = DefaultRoundFunc
	}
	groups, createQueryErr := newQuery()
	if createQueryErr != nil {
		maxProcs = -1
		status = QuotaUndefined
		err = createQueryErr
		return
	}

	quota, defined, getQuotaErr := groups.CPUQuota()
	if !defined || getQuotaErr != nil {
		maxProcs = -1
		status = QuotaUndefined
		err = getQuotaErr
		return
	}

	maxProcs = round(quota)
	if minValue > 0 && maxProcs < minValue {
		maxProcs = minValue
		status = QuotaMinUsed
		return
	}
	status = QuotaUsed
	return
}

type query interface {
	CPUQuota() (float64, bool, error)
}

func newQuery() (q query, err error) {
	q, err = cgroups.NewCGroups2ForCurrentProcess()
	if err == nil {
		return
	}
	if errors.Is(err, cgroups.ErrNotV2) {
		q, err = cgroups.NewCGroupsForCurrentProcess()
		return
	}
	return
}
