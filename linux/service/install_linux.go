package service

import (
	"fmt"
	"os"
	"os/exec"
)

func InstallService(listen string) (err error) {
	service := `
[Unit]
Description=CakeAgent Service
After=network.target

[Service]
Type=simple
Restart=on-failure
ExecStart=` + fmt.Sprintf("%s --listen=%s", os.Args[0], listen) + `
Environment=PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin/:/sbin

[Install]
WantedBy=multi-user.target
`

	servicePath := "/etc/systemd/system/cakeagent.service"

	if err := os.WriteFile(servicePath, []byte(service), 0644); err != nil {
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
