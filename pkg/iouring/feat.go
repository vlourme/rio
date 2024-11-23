package iouring

const (
	FeatSingleMMap uint32 = 1 << iota
	FeatNoDrop
	FeatSubmitStable
	FeatRWCurPos
	FeatCurPersonality
	FeatFastPoll
	FeatPoll32Bits
	FeatSQPollNonfixed
	FeatExtArg
	FeatNativeWorkers
	FeatRcrcTags
	FeatCQESkip
	FeatLinkedFile
	FeatRegRegRing
)
