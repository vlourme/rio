package process

type PriorityLeven int

const (
	NORM PriorityLeven = iota
	HIGH
	IDLE
	REALTIME
)
