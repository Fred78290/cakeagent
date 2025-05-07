package mount

import (
	"context"
	"fmt"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
)

func mount(_ context.Context, _ *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	return &cakeagent.CakeAgent_MountReply{
		Response: &cakeagent.CakeAgent_MountReply_Error{
			Error: "method Mount not supported on darwin",
		},
	}, nil
}

func umount(_ context.Context, _ *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	return &cakeagent.CakeAgent_MountReply{
		Response: &cakeagent.CakeAgent_MountReply_Error{
			Error: "method Umount not supported on darwin",
		},
	}, nil
}

func mountEndpoints(_ []string) error {
	return fmt.Errorf("method MountEndpoints not supported on darwin")
}
