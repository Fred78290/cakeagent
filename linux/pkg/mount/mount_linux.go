package mount

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/utils"
)

func mountEndpoint(name, target string, uid, gid int32, readonly bool) (err error) {
	// umount first to avoid "device or resource busy" error
	utils.Shell("umount", "-t", "virtiofs", name)

	// create target directory
	utils.Shell("mkdir", "-p", target)

	// change owner of target directory
	utils.Shell("chown", fmt.Sprintf("%d:%d", uid, gid), name)

	// mount virtiofs
	if _, _, err = utils.Shell("mount", "-t", "virtiofs", name, target); err != nil {
		// if mount failed, remove target directory
		utils.Shell("rm", "-rf", target)
	} else {
		// update /etc/fstab
		mode := "rw"
		if readonly {
			mode = "ro"
		}

		utils.Shell("sed", "-i", fmt.Sprintf("/%s/d", name), "/etc/fstab")
		utils.Shell("echo", fmt.Sprintf("%s %s virtiofs relatime,%s 0 0", name, target, mode), ">>", "/etc/fstab")
	}

	return
}

func mount(_ context.Context, request *cakeagent.MountRequest) (*cakeagent.MountReply, error) {
	mounts := make([]*cakeagent.MountVirtioFSReply, 0, len(request.Mounts))

	for _, m := range request.Mounts {
		// mount virtiofs
		if err := mountEndpoint(m.Name, m.Target, m.Uid, m.Gid, m.Readonly); err != nil {
			// if mount failed, remove target directory
			utils.Shell("rm", "-rf", m.Target)

			mounts = append(mounts, &cakeagent.MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.MountVirtioFSReply_Error{
					Error: err.Error(),
				},
			})
		} else {
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

func mountEndpoints(mounts []string) (err error) {
	for _, m := range mounts {
		options := strings.Split(m, ",")
		arguments := strings.Split(options[0], ":")
		name := arguments[0]
		target := arguments[1]
		uid := int32(0)
		gid := int32(0)
		readonly := false

		for _, option := range options[1:] {
			if option == "ro" {
				readonly = true
			} else if strings.HasPrefix(option, "gid=") {
				if val, err := strconv.Atoi(option[4:]); err == nil {
					uid = int32(val)
				}
			} else if strings.HasPrefix(option, "gid=") {
				if val, err := strconv.Atoi(option[4:]); err == nil {
					gid = int32(val)
				}
			}
		}

		if err = mountEndpoint(name, target, uid, gid, readonly); err != nil {
			return
		}
	}

	return
}
