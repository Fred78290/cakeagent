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
		fmt.Sprintf("<string>--listen=%s</string>", cfg.Address),
	}

	if cfg.CaCert != "" {
		args = append(args, fmt.Sprintf("<string>--ca-cert='%s'</string>", cfg.CaCert))
	}

	if cfg.TlsCert != "" {
		args = append(args, fmt.Sprintf("<string>--tls-cert='%s'</string>", cfg.TlsCert))
	}

	if cfg.TlsKey != "" {
		args = append(args, fmt.Sprintf("<string>--tls-key='%s'</string>", cfg.TlsKey))
	}

	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple Computer//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
	<dict>
		<key>Label</key>
			<string>com.aldunelabs.cakeagent</string>

		<key>ProgramArguments</key>
			<array>` + strings.Join(args, " ") + `</array>

		<key>KeepAlive</key>
			<dict>
				<key>SuccessfulExit</key>
					<false/>
			</dict>

		<key>RunAtLoad</key>
			<true/>

		<key>KeepAlive</key>
			<true/>

		<key>AbandonProcessGroup</key>
			<true/>

		<key>StandardErrorPath</key>
			<string>/Library/Logs/cakeagent.log</string>

		<key>StandardOutPath</key>
			<string>/Library/Logs/cakeagent.log</string>

		<key>ProcessType</key>
			<string>Background</string>

		<key>SoftResourceLimits</key>
			<dict>
				<key>NumberOfFiles</key>
					<integer>4096</integer>
			</dict>

		<key>EvironmentVariables</key>
			<dict>
				<key>PATH</key>
					<string>/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
			</dict>
	</dict>
</plist>`

	plistPath := "/Library/LaunchDaemons/com.aldunelabs.cakeagent.plist"

	if e := os.WriteFile(plistPath, []byte(plist), 0644); e != nil {
		err = fmt.Errorf("failed to write plist: %w", e)
	} else {
		cmd := exec.Command("launchctl", "load", plistPath)
		if output, e := cmd.CombinedOutput(); e != nil {
			err = fmt.Errorf("failed to load launch daemon: %w, output: %s", e, string(output))
		}
	}

	return
}
