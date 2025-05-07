package mount

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Fred78290/cakeagent/pkg/cakeagent"
	"github.com/Fred78290/cakeagent/pkg/utils"
	glog "github.com/sirupsen/logrus"
)

func mountEndpoint(name, target string, uid, gid int32, readonly, early bool) (err error) {
	var stderr string

	glog.Infof("Mounting %s to %s with uid=%d, gid=%d, readonly=%t", name, target, uid, gid, readonly)

	// umount first to avoid "device or resource busy" error
	utils.Shell("umount", "-t", "virtiofs", name)

	// create target directory
	utils.Shell("mkdir", "-p", target)

	// change owner of target directory
	utils.Shell("chown", fmt.Sprintf("%d:%d", uid, gid), name)

	// mount virtiofs
	if _, stderr, err = utils.Shell("mount", "-t", "virtiofs", name, target); !early && err != nil {
		glog.Errorf("Failed to mount %s to %s: %v\n%s", name, target, err, stderr)
		// if mount failed, remove target directory
		utils.Shell("rm", "-rf", target)
	} else {
		// update /etc/fstab
		mode := "rw"
		if readonly {
			mode = "ro"
		}

		utils.Shell("sed", "-i", fmt.Sprintf("/%s/d", name), "/etc/fstab")
		utils.Shell("bash", "-c", fmt.Sprintf("echo '%s %s virtiofs relatime,nofail,%s 0 0' >> /etc/fstab", name, target, mode))
	}

	return
}

func mount(_ context.Context, request *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	mounts := make([]*cakeagent.CakeAgent_MountReply_MountVirtioFSReply, 0, len(request.Mounts))

	for _, m := range request.Mounts {
		// mount virtiofs
		if err := mountEndpoint(m.Name, m.Target, m.Uid, m.Gid, m.Readonly, m.Early); err != nil {
			// if mount failed, remove target directory
			utils.Shell("rm", "-rf", m.Target)

			mounts = append(mounts, &cakeagent.CakeAgent_MountReply_MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.CakeAgent_MountReply_MountVirtioFSReply_Error{
					Error: err.Error(),
				},
			})
		} else {
			// update mounts
			mounts = append(mounts, &cakeagent.CakeAgent_MountReply_MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.CakeAgent_MountReply_MountVirtioFSReply_Success{
					Success: true,
				},
			})
		}
	}

	return &cakeagent.CakeAgent_MountReply{
		Response: &cakeagent.CakeAgent_MountReply_Success{
			Success: true,
		},
	}, nil
}

func umount(_ context.Context, request *cakeagent.CakeAgent_MountRequest) (*cakeagent.CakeAgent_MountReply, error) {
	mounts := make([]*cakeagent.CakeAgent_MountReply_MountVirtioFSReply, 0, len(request.Mounts))

	for _, m := range request.Mounts {
		if _, _, err := utils.Shell("umount", "-t", "virtiofs", m.Name); err != nil {
			mounts = append(mounts, &cakeagent.CakeAgent_MountReply_MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.CakeAgent_MountReply_MountVirtioFSReply_Error{
					Error: err.Error(),
				},
			})
		} else {
			// remove target directory
			utils.Shell("rm", "-rf", m.Target)

			// remove from /etc/fstab
			utils.Shell("sed", "-i", fmt.Sprintf("/%s/d", m.Name), "/etc/fstab")
			mounts = append(mounts, &cakeagent.CakeAgent_MountReply_MountVirtioFSReply{
				Name: m.Name,
				Response: &cakeagent.CakeAgent_MountReply_MountVirtioFSReply_Success{
					Success: true,
				},
			})
		}
	}

	return &cakeagent.CakeAgent_MountReply{
		Mounts: mounts,
		Response: &cakeagent.CakeAgent_MountReply_Success{
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

		if err = mountEndpoint(name, target, uid, gid, readonly, false); err != nil {
			return
		}
	}

	return
}
