package resize

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/samber/lo"
	"howett.net/plist"
)

type CandidatePartition struct {
	DiskName       string
	DiskSizeUnused int64
	PartitionName  string
}

type PhysicalStore struct {
	APFSPhysicalStores []string `plist:"APFSPhysicalStores"`
}

type ListVolumes struct {
	PhysicalDisks []PhysicalDisk `plist:"AllDisksAndPartitions"`
}

type PhysicalDisk struct {
	DeviceIdentifier string          `plist:"DeviceIdentifier"`
	Size             int64           `plist:"Size"`
	Partitions       []DiskPartition `plist:"Partitions"`
}

type DiskPartition struct {
	DeviceIdentifier string `plist:"DeviceIdentifier"`
	Size             int64  `plist:"Size"`
	Content          string `plist:"Content"`
}

var (
	ErrAlreadyResized        = errors.New("disk already seems to be resized")
	ErrUnableToRepairDisk    = errors.New("unable to repair disk")
	ErrNoPartitionFound      = errors.New("no disk found with APFS partition")
	ErrTooManyPartitionFound = errors.New("more than one disk found with APFS partition")
	ErrNoDiskFound           = errors.New("no disk found")
	ErrNoVolumes             = errors.New("no volumes found")
	ErrResizeContainerFailed = errors.New("resize container failed")
)

func executeCommand(input *string, command string, args ...string) ([]byte, error) {
	cmd := exec.Command(command, args...)

	if input != nil {
		cmd.Stdin = bytes.NewReader([]byte(*input))
	}
	stderrBuf := &bytes.Buffer{}
	cmd.Stderr = stderrBuf
	cmd.Env = os.Environ()

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute command: %w: %s", err, strings.Split(stderrBuf.String(), "\n")[0])
	}

	return output, nil
}

func getVolumes() (*ListVolumes, error) {
	var volumes ListVolumes

	// Obtain a list of physical disks with their partitions
	if output, err := executeCommand(nil, "diskutil", "list", "-plist", "physical"); err != nil {
		return nil, fmt.Errorf("failed to list physical disks: %w", err)
	} else if _, err = plist.Unmarshal(output, &volumes); err != nil {
		return nil, fmt.Errorf("failed to parse reason: %v", err)
	}

	return &volumes, nil
}

func getRootPhysicalStore() (*PhysicalStore, error) {
	var store PhysicalStore

	if output, err := executeCommand(nil, "diskutil", "info", "-plist", "/"); err != nil {
		return nil, fmt.Errorf("failed to get infos on /: %w", err)
	} else if _, err = plist.Unmarshal(output, &store); err != nil {
		return nil, fmt.Errorf("failed to parse reason: %v", err)
	}

	return &store, nil
}

func (volumes *ListVolumes) findResizeablePartition(rootDisk string) (result *CandidatePartition, err error) {
	var candidates []CandidatePartition

	for _, disk := range volumes.PhysicalDisks {
		unusedSize := disk.Size - lo.SumBy(disk.Partitions, func(partition DiskPartition) int64 {
			return partition.Size
		})

		for _, partition := range disk.Partitions {
			if partition.Content != "Apple_APFS" && rootDisk != partition.DeviceIdentifier {
				continue
			}

			candidates = append(candidates, CandidatePartition{
				DiskName:       disk.DeviceIdentifier,
				DiskSizeUnused: unusedSize,
				PartitionName:  partition.DeviceIdentifier,
			})
		}
	}

	if len(candidates) == 0 {
		err = ErrNoPartitionFound
	} else if len(candidates) > 1 {
		err = ErrTooManyPartitionFound // This is a bug in the code, as we should only have one candidate
	} else {
		result = &candidates[0]
	}

	return
}

func resizeDisk() (err error) {
	var volumes *ListVolumes
	var rootVolume *PhysicalStore
	var partition *CandidatePartition
	yes := "yes"

	if rootVolume, err = getRootPhysicalStore(); err != nil {
		err = ErrNoDiskFound
	} else if volumes, err = getVolumes(); err != nil {
		err = ErrNoVolumes
	} else if partition, err = volumes.findResizeablePartition(rootVolume.APFSPhysicalStores[0]); err != nil {
		err = ErrNoPartitionFound
	} else if partition.DiskSizeUnused <= 4096*10 {
		err = ErrAlreadyResized
	} else if _, err = executeCommand(&yes, "diskutil", "repairDisk", partition.DiskName); err != nil {
		err = ErrUnableToRepairDisk
	} else if _, err = executeCommand(nil, "diskutil", "apfs", "resizeContainer", partition.PartitionName, "0"); err != nil {
		err = ErrResizeContainerFailed
	}

	return
}
