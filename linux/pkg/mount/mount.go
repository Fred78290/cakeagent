package mount

import (
	"context"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
)

func Mount(ctx context.Context, request *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	return mount(ctx, request)
}

func Umount(ctx context.Context, request *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	return umount(ctx, request)
}
