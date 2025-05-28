package console

import "os"

const virtioPortPath = "/dev/tty.cake-console"

func syslogFile() (path string, err error) {
	path = "/var/log/syslog"

	if _, err = os.Stat(path); os.IsNotExist(err) {
		path = "/var/log/messages"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return "", err
		}
	}

	return path, nil
}
