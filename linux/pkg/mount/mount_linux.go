package mount

import (
	"context"
	"fmt"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/utils"
)

func mount(_ context.Context, request *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	mounts := make([]*cakeagent.MountVirtioFSReply, 0, len(request.Mounts))

	for _, m := range request.Mounts {
		// umount first to avoid "device or resource busy" error
		utils.Shell("umount", "-t", "virtiofs", m.Name)

		// create target directory
		utils.Shell("mkdir", "-p", m.Target)

		// change owner of target directory
		utils.Shell("chown", fmt.Sprintf("%d:%d", m.Uid, m.Gid), m.Target)

		// mount virtiofs
		if _, _, err := utils.Shell("mount", "-t", "virtiofs", m.Name, m.Target); err != nil {
			// if mount failed, remove target directory
			utils.Shell("rm", "-rf", m.Target)

			mounts = append(mounts, &cakeagent.MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.MountVirtioFSReply_Error{
					Error: err.Error(),
				},
			})
		} else {
			// update /etc/fstab
			mode := "rw"
			if m.Readonly {
				mode = "ro"
			}

			utils.Shell("sed", "-i", fmt.Sprintf("/%s/d", m.Name), "/etc/fstab")
			utils.Shell("echo", fmt.Sprintf("%s %s virtiofs relatime,%s 0 0", m.Name, m.Target, mode), ">>", "/etc/fstab")

			// update mounts
			mounts = append(mounts, &cakeagent.MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.MountVirtioFSReply_Success{
					Success: true,
				},
			})
		}
	}

	return &cakeagent.MountReply{
		Response: &cakeagent.MountReply_Success{
			Success: true,
		},
	}, nil
}

func umount(_ context.Context, request *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	mounts := make([]*cakeagent.MountVirtioFSReply, 0, len(request.Mounts))

	for _, m := range request.Mounts {
		if _, _, err := utils.Shell("umount", "-t", "virtiofs", m.Name); err != nil {
			mounts = append(mounts, &cakeagent.MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.MountVirtioFSReply_Error{
					Error: err.Error(),
				},
			})
		} else {
			// remove target directory
			utils.Shell("rm", "-rf", m.Target)

			// remove from /etc/fstab
			utils.Shell("sed", "-i", fmt.Sprintf("/%s/d", m.Name), "/etc/fstab")
			mounts = append(mounts, &cakeagent.MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.MountVirtioFSReply_Success{
					Success: true,
				},
			})
		}
	}

	return &cakeagent.MountReply{
		Mounts: mounts,
		Response: &cakeagent.MountReply_Success{
			Success: true,
		},
	}, nil
}
