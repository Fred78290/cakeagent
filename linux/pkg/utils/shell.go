package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Shell execute local command
func Shell(args ...string) (stdOut string, stdErr string, err error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(args[0], args[1:]...)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	stdOut = strings.TrimSpace(stdout.String())
	stdErr = strings.TrimSpace(stderr.String())

	if err == nil {
		if cmd.ProcessState.ExitCode() != 0 {
			err = fmt.Errorf("mount failed: %s: %d", stdErr, cmd.ProcessState.ExitCode())
		}
	}

	return
}

// ToJSON serialize interface to json
func ToJSON(v any) string {
	if v == nil {
		return ""
	}

	b, _ := json.Marshal(v)

	return string(b)
}
