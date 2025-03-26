//go:build linux

package liburing

type ProbeOp struct {
	Op    uint8
	Res   uint8
	Flags uint16
	Res2  uint32
}

const (
	probeOpsSize = 256
)

const IO_URING_OP_SUPPORTED uint16 = 1 << 0

type Probe struct {
	LastOp uint8
	OpsLen uint8
	Res    uint16
	Res2   [3]uint32
	Ops    [probeOpsSize]ProbeOp
}

func (p *Probe) IsSupported(op uint8) bool {
	for i := uint8(0); i < p.OpsLen; i++ {
		if p.Ops[i].Op != op {
			continue
		}
		return p.Ops[i].Flags&IO_URING_OP_SUPPORTED != 0
	}
	return false
}

const probeEntries = 2

func GetProbe() (*Probe, error) {
	ring, err := New(WithEntries(probeEntries))
	if err != nil {
		return nil, err
	}
	probe, probeErr := ring.Probe()
	if probeErr != nil {
		_ = ring.Close()
		return nil, probeErr
	}
	_ = ring.Close()
	return probe, nil
}
