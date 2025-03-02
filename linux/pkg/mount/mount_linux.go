package mount

import (
	"context"
	"fmt"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/utils"
)

func mount(_ context.Context, request *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	for _, m := range request.Mounts {
		utils.Shell("umount", "-t", "virtiofs", m.Name)
		utils.Shell("mkdir", "-p", m.Target)
		utils.Shell("chown", fmt.Sprintf("%d:%d", m.Uid, m.Gid), m.Target)
		utils.Shell("mount", "-t", "virtiofs", m.Name, m.Target)
		utils.Shell("sed", "-i", fmt.Sprintf("/%s/d", m.Name), "/etc/fstab")
		utils.Shell("echo", fmt.Sprintf("%s %s virtiofs relatime,uid=%d,gid=%d 0 0", m.Name, m.Target, m.Uid, m.Gid), ">>", "/etc/fstab")
	}

	return &cakeagent.MountReply{
		Response: &cakeagent.MountReply_Success{
			Success: true,
		},
	}, nil
}

func umount(_ context.Context, request *cakeagent.UmountRequest) (*cakeagent.MountReply, error) {
	for _, m := range request.Mounts {
		utils.Shell("umount", "-t", "virtiofs", m.Name)
		utils.Shell("sed", "-i", fmt.Sprintf("/%s/d", m.Name), "/etc/fstab")
	}

	return &cakeagent.MountReply{
		Response: &cakeagent.MountReply_Success{
			Success: true,
		},
	}, nil
}
