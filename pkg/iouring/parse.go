package iouring

import "strings"

func ParseSetupFlags(s string) uint32 {
	s = strings.TrimSpace(s)
	s = strings.ToUpper(s)
	switch s {
	case "IORING_SETUP_IOPOLL":
		return SetupIOPoll
	case "IORING_SETUP_SQPOLL":
		return SetupSQPoll
	case "IORING_SETUP_SQ_AFF":
		return SetupSQAff
	case "IORING_SETUP_CQSIZE":
		return SetupCQSize
	case "IORING_SETUP_CLAMP":
		return SetupClamp
	case "IORING_SETUP_ATTACH_WQ":
		return SetupAttachWQ
	case "IORING_SETUP_R_DISABLED":
		return SetupRDisabled
	case "IORING_SETUP_SUBMIT_ALL":
		return SetupSubmitAll
	case "IORING_SETUP_COOP_TASKRUN":
		return SetupCoopTaskRun
	case "IORING_SETUP_TASKRUN_FLAG":
		return SetupTaskRunFlag
	case "IORING_SETUP_SQE128":
		return SetupSQE128
	case "IORING_SETUP_CQE32":
		return SetupCQE32
	case "IORING_SETUP_SINGLE_ISSUER":
		return SetupSingleIssuer
	case "IORING_SETUP_DEFER_TASKRUN":
		return SetupDeferTaskRun
	case "IORING_SETUP_NO_MMAP":
		return SetupNoMmap
	case "IORING_SETUP_REGISTERED_FD_ONLY":
		return SetupRegisteredFdOnly
	case "IORING_SETUP_NO_SQARRAY":
		return SetupNoSQArray
	default:
		return 0
	}
}
