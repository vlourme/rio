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
	groups, err := newQuery()
	if err != nil {
		maxProcs = -1
		status = QuotaUndefined
		return
	}

	quota, defined, err := groups.CPUQuota()
	if !defined || err != nil {
		maxProcs = -1
		status = QuotaUndefined
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
	return nil, err
}
