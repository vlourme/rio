//go:build linux

package cgroups

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	mountInfoSep               = " "
	mountInfoOptsSep           = ","
	mountInfoOptionalFieldsSep = "-"
)

const (
	miFieldIdMountId = iota
	miFieldIdParentId
	miFieldIdDeviceId
	miFieldIdRoot
	miFieldIdMountPoint
	miFieldIdOptions
	miFieldIdOptionalFields
	miFieldCountFirstHalf
)

const (
	miFieldOffsetFSType = iota
	miFieldOffsetMountSource
	miFieldOffsetSuperOptions
	miFieldCountSecondHalf
)

const miFieldCountMin = miFieldCountFirstHalf + miFieldCountSecondHalf

type MountPoint struct {
	MountID        int
	ParentID       int
	DeviceID       string
	Root           string
	MountPoint     string
	Options        []string
	OptionalFields []string
	FSType         string
	MountSource    string
	SuperOptions   []string
}

func NewMountPointFromLine(line string) (mp *MountPoint, err error) {
	fields := strings.Split(line, mountInfoSep)

	if len(fields) < miFieldCountMin {
		err = mountPointFormatInvalidError{line}
		return
	}

	mountId, parseMountIdErr := strconv.Atoi(fields[miFieldIdMountId])
	if parseMountIdErr != nil {
		err = parseMountIdErr
		return
	}

	parentId, parseParentIdErr := strconv.Atoi(fields[miFieldIdParentId])
	if parseParentIdErr != nil {
		err = parseParentIdErr
		return
	}

	for i, field := range fields[miFieldIdOptionalFields:] {
		if field == mountInfoOptionalFieldsSep {
			fsTypeStart := miFieldIdOptionalFields + i + 1
			fields = strings.SplitN(line, mountInfoSep, fsTypeStart+miFieldCountSecondHalf)
			if len(fields) != fsTypeStart+miFieldCountSecondHalf {
				return nil, mountPointFormatInvalidError{line}
			}

			miFieldIDFSType := miFieldOffsetFSType + fsTypeStart
			miFieldIDMountSource := miFieldOffsetMountSource + fsTypeStart
			miFieldIDSuperOptions := miFieldOffsetSuperOptions + fsTypeStart

			mp = &MountPoint{
				MountID:        mountId,
				ParentID:       parentId,
				DeviceID:       fields[miFieldIdDeviceId],
				Root:           fields[miFieldIdRoot],
				MountPoint:     fields[miFieldIdMountPoint],
				Options:        strings.Split(fields[miFieldIdOptions], mountInfoOptsSep),
				OptionalFields: fields[miFieldIdOptionalFields:(fsTypeStart - 1)],
				FSType:         fields[miFieldIDFSType],
				MountSource:    fields[miFieldIDMountSource],
				SuperOptions:   strings.Split(fields[miFieldIDSuperOptions], mountInfoOptsSep),
			}
			return
		}
	}

	err = mountPointFormatInvalidError{line}
	return
}

func (mp *MountPoint) Translate(absPath string) (path string, err error) {
	relPath, relErr := filepath.Rel(mp.Root, absPath)
	if relErr != nil {
		err = relErr
		return
	}
	if relPath == ".." || strings.HasPrefix(relPath, "../") {
		err = pathNotExposedFromMountPointError{
			mountPoint: mp.MountPoint,
			root:       mp.Root,
			path:       absPath,
		}
		return
	}
	path = filepath.Join(mp.MountPoint, relPath)
	return
}

func parseMountInfo(procPathMountInfo string, newMountPoint func(*MountPoint) error) (err error) {
	mountInfoFile, openErr := os.Open(procPathMountInfo)
	if openErr != nil {
		err = openErr
		return
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(mountInfoFile)

	scanner := bufio.NewScanner(mountInfoFile)

	for scanner.Scan() {
		mountPoint, newMountPointErr := NewMountPointFromLine(scanner.Text())
		if newMountPointErr != nil {
			err = newMountPointErr
			return
		}
		if err = newMountPoint(mountPoint); err != nil {
			return err
		}
	}
	err = scanner.Err()
	return
}
