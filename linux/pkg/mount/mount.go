package mount

import (
	"context"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
)

func Mount(ctx context.Context, request *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	return mount(ctx, request)
}

func Umount(ctx context.Context, request *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	return umount(ctx, request)
}

func MountEndpoints(mounts []string) error {
	return mountEndpoints(mounts)
}
