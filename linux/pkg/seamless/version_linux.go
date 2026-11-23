//go:build linux

package seamless

import (
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// RunningCompositor is a compositor process and the version of the binary it was started from. That is not always the
// installed one: an agent updated in place replaces the file, and a compositor that was already running keeps the old one
// until it is restarted.
type RunningCompositor struct {
	PID     int    `json:"pid"`
	Version string `json:"version"`
	User    string `json:"user,omitempty"`
}

// processOwner is the user name of a process, its numeric id when the account is unknown.
func processOwner(pid string) string {
	info, err := os.Stat("/proc/" + pid)

	if err != nil {
		return ""
	}

	st, ok := info.Sys().(*syscall.Stat_t)

	if !ok {
		return ""
	}

	uid := strconv.FormatUint(uint64(st.Uid), 10)

	if u, err := user.LookupId(uid); err == nil {
		return u.Username
	}

	return uid
}

// RunningVersions lists the compositors running now (every user's) with the version of the binary each runs. It reads
// /proc/<pid>/exe, which still gives the file of a compositor whose binary was replaced since, so it needs the rights to
// read another user's processes (the agent is root): a compositor it cannot read is reported with UnknownVersion.
func RunningVersions() []RunningCompositor {
	entries, err := os.ReadDir("/proc")

	if err != nil {
		return nil
	}

	var found []RunningCompositor

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())

		if err != nil {
			continue
		}

		comm, err := os.ReadFile("/proc/" + e.Name() + "/comm")

		// comm holds the first 15 characters of the name, which is what BinaryName has.
		if err != nil || strings.TrimSpace(string(comm)) != BinaryName {
			continue
		}

		version := UnknownVersion

		if data, err := os.ReadFile("/proc/" + e.Name() + "/exe"); err == nil {
			version = VersionOf(data)
		}

		found = append(found, RunningCompositor{PID: pid, Version: version, User: processOwner(e.Name())})
	}

	sort.Slice(found, func(i, j int) bool { return found[i].PID < found[j].PID })

	return found
}
