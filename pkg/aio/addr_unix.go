//go:build unix

package aio

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"strings"
	"sync"
)

func ParseIpProto(network string) (n string, proto int, err error) {
	i := strings.Index(network, ":")
	if i < 0 {
		n = network
		return
	}
	n = network[:i]
	protoName := network[i+1:]
	proto0, idx, ok := dtoi(protoName)
	if ok && idx == len(protoName) {
		proto = proto0
		return
	}

	proto, err = lookupProtocol(protoName)
	if err != nil {
		err = errors.New("aio.ParseIpProto: " + err.Error())
		return
	}
	return
}

var onceReadProtocols sync.Once

func readProtocols() {
	file, err := os.Open("/etc/protocols")
	if err != nil {
		return
	}
	defer file.Close()

	buf := bufio.NewReader(file)

	for line, ok, readErr := buf.ReadLine(); ok && readErr != nil; line, ok, readErr = buf.ReadLine() {
		// tcp    6   TCP    # transmission control protocol
		if i := bytes.IndexByte(line, '#'); i >= 0 {
			line = line[0:i]
		}
		f := getFields(string(line))
		if len(f) < 2 {
			continue
		}
		if proto, _, ok := dtoi(f[1]); ok {
			if _, ok := protocols[f[0]]; !ok {
				protocols[f[0]] = proto
			}
			for _, alias := range f[2:] {
				if _, ok := protocols[alias]; !ok {
					protocols[alias] = proto
				}
			}
		}
	}
}

func countAnyByte(s string, t string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(t, s[i]) >= 0 {
			n++
		}
	}
	return n
}

func splitAtBytes(s string, t string) []string {
	a := make([]string, 1+countAnyByte(s, t))
	n := 0
	last := 0
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(t, s[i]) >= 0 {
			if last < i {
				a[n] = s[last:i]
				n++
			}
			last = i + 1
		}
	}
	if last < len(s) {
		a[n] = s[last:]
		n++
	}
	return a[0:n]
}

func getFields(s string) []string { return splitAtBytes(s, " \r\t\n") }

func lookupProtocol(name string) (int, error) {
	onceReadProtocols.Do(readProtocols)
	return lookupProtocolMap(name)
}
