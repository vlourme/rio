package iouring

const (
	RestrictionRegisterOp uint32 = iota
	RestrictionSQEOp
	RestrictionSQEFlagsAllowed
	RestrictionSQEFlagsRequired
	RestrictionLast
)

// liburing: io_uring_restriction
type Restriction struct {
	OpCode uint16
	// union {
	// 	__u8 register_op; /* IORING_RESTRICTION_REGISTER_OP */
	// 	__u8 sqe_op;      /* IORING_RESTRICTION_SQE_OP */
	// 	__u8 sqe_flags;   /* IORING_RESTRICTION_SQE_FLAGS_* */
	// };
	OpFlags uint8
	Resv    uint8
	Resv2   [3]uint32
}
