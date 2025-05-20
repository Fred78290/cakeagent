package event

import (
	"time"

	"github.com/lima-vm/lima/pkg/guestagent"
)

func NewAgent(tick time.Duration) (guestagent.Agent, error) {
	return newAgent(tick)
}
