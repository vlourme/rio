//go:build linux

package cgroups

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

const (
	sep       = ":"
	subsysSep = ","
)

const (
	csFieldIdId = iota
	csFieldIdSubsystems
	csFieldIdName
	csFieldCount
)

type Subsystem struct {
	Id         int
	Subsystems []string
	Name       string
}

func NewSubsystemFromLine(line string) (v *Subsystem, err error) {
	fields := strings.SplitN(line, sep, csFieldCount)

	if len(fields) != csFieldCount {
		err = cgroupSubsysFormatInvalidError{line}
		return
	}

	id, parseErr := strconv.Atoi(fields[csFieldIdId])
	if parseErr != nil {
		err = parseErr
		return
	}

	v = &Subsystem{
		Id:         id,
		Subsystems: strings.Split(fields[csFieldIdSubsystems], subsysSep),
		Name:       fields[csFieldIdName],
	}

	return
}

func parseSubsystems(procPathCGroup string) (subsystems map[string]*Subsystem, err error) {
	cgroupFile, openErr := os.Open(procPathCGroup)
	if openErr != nil {
		err = openErr
		return
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(cgroupFile)

	scanner := bufio.NewScanner(cgroupFile)
	subsystems = make(map[string]*Subsystem)
	for scanner.Scan() {
		subsystem, newSubsystemErr := NewSubsystemFromLine(scanner.Text())
		if newSubsystemErr != nil {
			err = newSubsystemErr
			return
		}
		for _, item := range subsystem.Subsystems {
			subsystems[item] = subsystem
		}
	}
	if err = scanner.Err(); err != nil {
		return
	}
	return
}
