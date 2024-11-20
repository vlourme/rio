package process

type PriorityLevel int

const (
	NORM PriorityLevel = iota
	HIGH
	IDLE
	REALTIME
)
