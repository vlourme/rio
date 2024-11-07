//go:build linux

package cgroups

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

const (
	fsType              = "cgroup"
	subsystemCPU        = "cpu"
	cpuCfsQuotaUsParam  = "cpu.cfs_quota_us"
	cpuCfsPeriodUsParam = "cpu.cfs_period_us"
)

const (
	procPathCGroup    = "/proc/self/cgroup"
	procPathMountInfo = "/proc/self/mountinfo"
)

type CGroups map[string]*CGroup

func NewCGroups(procPathMountInfo, procPathCGroup string) (groups CGroups, err error) {
	cgroupSubsystems, parseCGroupSubsysErr := parseSubsystems(procPathCGroup)
	if parseCGroupSubsysErr != nil {
		err = parseCGroupSubsysErr
		return
	}
	groups = make(CGroups)
	newMountPoint := func(mp *MountPoint) (err error) {
		if mp.FSType != fsType {
			return
		}
		for _, opt := range mp.SuperOptions {
			subsys, exists := cgroupSubsystems[opt]
			if !exists {
				continue
			}

			cgroupPath, translateErr := mp.Translate(subsys.Name)
			if translateErr != nil {
				err = translateErr
				return
			}
			groups[opt] = NewCGroup(cgroupPath)
		}
		return
	}
	if parseMountErr := parseMountInfo(procPathMountInfo, newMountPoint); parseMountErr != nil {
		err = parseMountErr
		return
	}
	return
}

func NewCGroupsForCurrentProcess() (CGroups, error) {
	return NewCGroups(procPathMountInfo, procPathCGroup)
}

func (cg CGroups) CPUQuota() (quota float64, ok bool, err error) {
	cpuCGroup, exists := cg[subsystemCPU]
	if !exists {
		quota = -1
		return
	}

	cfsQuotaUs, readQuotaUsErr := cpuCGroup.readInt(cpuCfsQuotaUsParam)
	if defined := cfsQuotaUs > 0; readQuotaUsErr != nil || !defined {
		quota = -1
		ok = defined
		err = readQuotaUsErr
		return
	}

	cfsPeriodUs, readPeriodUsErr := cpuCGroup.readInt(cpuCfsPeriodUsParam)
	if defined := cfsPeriodUs > 0; readPeriodUsErr != nil || !defined {
		quota = -1
		ok = defined
		err = readPeriodUsErr
		return
	}

	quota = float64(cfsQuotaUs) / float64(cfsPeriodUs)
	ok = true
	return
}

const (
	cPUMaxV2              = "cpu.max"
	fsTypeV2              = "cgroup2"
	mountPointV2          = "/sys/fs/cgroup"
	cpuMaxDefaultPeriodV2 = 100000
	cpuMaxQuotaMaxV2      = "max"
)

const (
	cpuMaxQuotaIndexV2 = iota
	cpuMaxPeriodIndexV2
)

var ErrNotV2 = errors.New("not using cgroups2")

type CGroupsV2 struct {
	mountPoint string
	groupPath  string
	cpuMaxFile string
}

func NewCGroups2ForCurrentProcess() (*CGroupsV2, error) {
	return newCGroups2From(procPathMountInfo, procPathCGroup)
}

func newCGroups2From(mountInfoPath, procPathCGroup string) (groups *CGroupsV2, err error) {
	isV2, isV2Err := isCGroupV2(mountInfoPath)
	if isV2Err != nil {
		err = isV2Err
		return
	}

	if !isV2 {
		err = ErrNotV2
		return
	}

	subsystems, parseCGroupSubsysErr := parseSubsystems(procPathCGroup)
	if parseCGroupSubsysErr != nil {
		err = parseCGroupSubsysErr
		return
	}

	var subsysV2 *Subsystem
	for _, subsys := range subsystems {
		if subsys.Id == 0 {
			subsysV2 = subsys
			break
		}
	}

	if subsysV2 == nil {
		err = ErrNotV2
		return
	}
	groups = &CGroupsV2{
		mountPoint: mountPointV2,
		groupPath:  subsysV2.Name,
		cpuMaxFile: cPUMaxV2,
	}
	return
}

func isCGroupV2(procPathMountInfo string) (isV2 bool, err error) {
	newMountPoint := func(mp *MountPoint) error {
		isV2 = isV2 || (mp.FSType == fsTypeV2 && mp.MountPoint == mountPointV2)
		return nil
	}
	if parseMountInfoErr := parseMountInfo(procPathMountInfo, newMountPoint); parseMountInfoErr != nil {
		err = parseMountInfoErr
		return
	}
	return
}

func (cg *CGroupsV2) CPUQuota() (quota float64, ok bool, err error) {
	cpuMaxParams, openErr := os.Open(path.Join(cg.mountPoint, cg.groupPath, cg.cpuMaxFile))
	if openErr != nil {
		if os.IsNotExist(err) {
			quota = -1
			return
		}
		quota = -1
		return
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(cpuMaxParams)

	scanner := bufio.NewScanner(cpuMaxParams)
	if scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || len(fields) > 2 {
			quota = -1
			err = errors.New("invalid format")
			return
		}

		if fields[cpuMaxQuotaIndexV2] == cpuMaxQuotaMaxV2 {
			quota = -1
			return
		}

		maxQuotaIndex, maxQuotaIndexErr := strconv.Atoi(fields[cpuMaxQuotaIndexV2])
		if maxQuotaIndexErr != nil {
			quota = -1
			err = maxQuotaIndexErr
			return
		}

		var period int
		if len(fields) == 1 {
			period = cpuMaxDefaultPeriodV2
		} else {
			period, err = strconv.Atoi(fields[cpuMaxPeriodIndexV2])
			if err != nil {
				quota = -1
				return
			}

			if period == 0 {
				quota = -1
				err = errors.New("zero value for period is not allowed")
				return
			}
		}

		quota = float64(maxQuotaIndex) / float64(period)
		ok = true
		return
	}

	if err = scanner.Err(); err != nil {
		quota = -1
		return
	}
	err = io.ErrUnexpectedEOF
	return
}
