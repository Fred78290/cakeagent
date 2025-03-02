package mount

import (
	"context"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
)

func mount(_ context.Context, _ *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	return &cakeagent.MountReply{
		Response: &cakeagent.MountReply_Error{
			Error: "method Mount not supported on darwin",
		},
	}, nil
}

func umount(_ context.Context, _ *cakeagent.UmountRequest) (*cakeagent.MountReply, error) {
	return &cakeagent.MountReply{
		Response: &cakeagent.MountReply_Error{
			Error: "method Umount not supported on darwin",
		},
	}, nil
}
