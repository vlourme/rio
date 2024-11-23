package iouring

// liburing: io_uring_probe_op
type ProbeOp struct {
	Op    uint8
	Res   uint8
	Flags uint16
	Res2  uint32
}

const (
	probeOpsSize = 256
	probeEntries = 2
)

// liburing: io_uring_probe
type Probe struct {
	LastOp uint8
	OpsLen uint8
	Res    uint16
	Res2   [3]uint32
	Ops    [probeOpsSize]ProbeOp
}

// liburing: io_uring_opcode_supported - https://manpages.debian.org/unstable/liburing-dev/io_uring_opcode_supported.3.en.html
func (p Probe) IsSupported(op uint8) bool {
	for i := uint8(0); i < p.OpsLen; i++ {
		if p.Ops[i].Op != op {
			continue
		}

		return p.Ops[i].Flags&opSupported != 0
	}

	return false
}

// liburing: io_uring_get_probe - https://manpages.debian.org/unstable/liburing-dev/io_uring_get_probe.3.en.html
func GetProbe() (*Probe, error) {
	ring, err := CreateRing(probeEntries)
	if err != nil {
		return nil, err
	}

	probe, err := ring.GetProbeRing()
	if err != nil {
		return nil, err
	}
	ring.QueueExit()

	return probe, nil
}

// liburing: io_uring_get_probe_ring
func (ring *Ring) GetProbeRing() (*Probe, error) {
	probe := &Probe{}
	_, err := ring.RegisterProbe(probe, probeOpsSize)
	if err != nil {
		return nil, err
	}

	return probe, nil
}
