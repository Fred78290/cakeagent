package service

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Fred78290/cakeagent/cmd/types"
)

func InstallService(cfg *types.Config) (err error) {
	args := []string{
		os.Args[0],
		fmt.Sprintf("--listen=%s", cfg.Address),
	}

	if cfg.CaCert != "" {
		args = append(args, fmt.Sprintf("--ca-cert=%s", cfg.CaCert))
	}

	if cfg.TlsCert != "" {
		args = append(args, fmt.Sprintf("--tls-cert=%s", cfg.TlsCert))
	}

	if cfg.TlsKey != "" {
		args = append(args, fmt.Sprintf("--tls-key=%s", cfg.TlsKey))
	}

	defaultEnv := ""

	service := `
[Unit]
Description=CakeAgent Service
After=network.target

[Service]
Type=simple
Restart=on-failure
EnvironmentFile=-/etc/default/cakeagent
Environment=PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin/:/sbin
User=root
ExecStart=` + strings.Join(args, " ") + `

[Install]
WantedBy=multi-user.target
`

	servicePath := "/etc/systemd/system/cakeagent.service"

	if err := os.WriteFile("/etc/default/cakeagent", []byte(defaultEnv), 0644); err != nil {
		err = fmt.Errorf("Failed to write env file: %v", err)
	} else if err = os.WriteFile(servicePath, []byte(service), 0644); err != nil {
		err = fmt.Errorf("Failed to write service file: %v", err)
	} else {
		cmds := [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "cakeagent"},
			{"systemctl", "start", "cakeagent"},
		}

		for _, cmdArgs := range cmds {
			cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)

			if output, err := cmd.CombinedOutput(); err != nil {
				err = fmt.Errorf("Command %v failed: %v, Output: %s", cmdArgs, err, output)
				break
			}
		}
	}

	return
}
