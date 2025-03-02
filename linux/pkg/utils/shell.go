package utils

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Shell execute local command
func Shell(args ...string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.Command(args[0], args[1:]...)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), fmt.Errorf("%s, %s", err.Error(), strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), nil
}
