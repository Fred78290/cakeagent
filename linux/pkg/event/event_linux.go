package event

import (
	"time"

	"github.com/lima-vm/lima/pkg/guestagent"
)

func newAgent(tick time.Duration) (guestagent.Agent, error) {
	newTicker := func() (<-chan time.Time, func()) {
		// TODO: use an equivalent of `bpftrace -e 'tracepoint:syscalls:sys_*_bind { printf("tick\n"); }')`,
		// without depending on `bpftrace` binary.
		// The agent binary will need CAP_BPF file cap.
		ticker := time.NewTicker(tick)
		return ticker.C, ticker.Stop
	}

	return guestagent.New(newTicker, tick*20)
}
