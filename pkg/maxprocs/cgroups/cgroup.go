//go:build linux

package cgroups

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type CGroup struct {
	path string
}

func NewCGroup(path string) *CGroup {
	return &CGroup{path: path}
}

func (cg *CGroup) Path() string {
	return cg.path
}

func (cg *CGroup) ParamPath(param string) string {
	return filepath.Join(cg.path, param)
}

func (cg *CGroup) readFirstLine(param string) (line string, err error) {
	paramFile, openErr := os.Open(cg.ParamPath(param))
	if openErr != nil {
		err = openErr
		return
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(paramFile)

	scanner := bufio.NewScanner(paramFile)
	if scanner.Scan() {
		line = scanner.Text()
		return
	}
	if err = scanner.Err(); err != nil {
		return
	}
	err = io.ErrUnexpectedEOF
	return
}

func (cg *CGroup) readInt(param string) (n int, err error) {
	text, readErr := cg.readFirstLine(param)
	if readErr != nil {
		err = readErr
		return
	}
	n, err = strconv.Atoi(text)
	return
}

type cgroupSubsysFormatInvalidError struct {
	line string
}

type mountPointFormatInvalidError struct {
	line string
}

type pathNotExposedFromMountPointError struct {
	mountPoint string
	root       string
	path       string
}

func (err cgroupSubsysFormatInvalidError) Error() string {
	return fmt.Sprintf("cgroup: invalid format for Subsystem: %q", err.line)
}

func (err mountPointFormatInvalidError) Error() string {
	return fmt.Sprintf("cgroup: invalid format for MountPoint: %q", err.line)
}

func (err pathNotExposedFromMountPointError) Error() string {
	return fmt.Sprintf("cgroup: path %q is not a descendant of mount point root %q and cannot be exposed from %q", err.path, err.root, err.mountPoint)
}
