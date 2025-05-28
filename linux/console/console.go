package console

import (
	"context"
	"errors"
	"io"
	os "os"
	"time"

	"github.com/Fred78290/cakeagent/pkg/utils"
	glog "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func NewConsole() *Console {
	var err error
	path := ""

	if _, err = os.Stat(virtioPortPath); os.IsNotExist(err) {
		return nil
	}

	glog.Debugf("Console virtio port found at %s", virtioPortPath)

	if path, err = syslogFile(); err != nil {
		return nil
	}

	glog.Debugf("Syslog file found at %s", path)

	return &Console{
		Address:      virtioPortPath,
		SyslogFile:   path,
		lastSizeSeen: -1,
		lastModified: time.Time{},
	}
}

type Console struct {
	Address      string    `json:"address"`
	SyslogFile   string    `json:"syslog_file,omitempty"`
	lastSizeSeen int64     `json:"-"`
	lastModified time.Time `json:"-"`
}

func (c *Console) Forward(ctx context.Context) {
	var file *os.File
	var err error

	// Implement the logic to forward console messages
	// This could involve reading from the console device and writing to a network socket or another endpoint
	// For now, this is a placeholder function
	if file, err = os.OpenFile(c.Address, os.O_WRONLY, 0666); err != nil {
		// Could not open the console device
		glog.Errorf("Error opening console device %s: %v", c.Address, err)
		return
	}

	defer file.Close()

	glog.Debugf("Forwarding console messages from %s to syslog file %s", c.Address, c.SyslogFile)

	c.copyBuffer(ctx, file) // Forward messages from syslog to the console device
}

func (c *Console) syslogChanged(console *os.File) (changed bool, newConsole *os.File, err error) {
	var fileInfo os.FileInfo

	if fileInfo, err = os.Stat(c.SyslogFile); err != nil {
		return false, console, err
	}

	if console == nil {
		if console, err = os.OpenFile(c.SyslogFile, os.O_RDONLY|unix.O_NONBLOCK, 0666); err != nil {
			glog.Errorf("Error opening syslog file %s: %v", c.SyslogFile, err)
			return false, nil, err
		}
	}

	// If this is the first time we are checking the file, we initialize lastSizeSeen and lastModified
	// to the current file size and modification time
	if c.lastSizeSeen < 0 {
		glog.Debugf("Initializing console syslog file %s with size %d and modified at %s", c.SyslogFile, fileInfo.Size(), fileInfo.ModTime())

		c.lastSizeSeen = 0
		c.lastModified = fileInfo.ModTime()

		return true, console, nil
	}

	if fileInfo.ModTime().After(c.lastModified) {
		glog.Debugf("Syslog file %s has changed: size %d, modified at %s", c.SyslogFile, fileInfo.Size(), fileInfo.ModTime())

		c.lastModified = fileInfo.ModTime()

		// If the file size has changed, we consider it as a logrotation or new log entry
		if fileInfo.Size() <= c.lastSizeSeen {
			glog.Debugf("Syslog file %s has been rotated or truncated, resetting lastSizeSeen", c.SyslogFile)

			// If the file has been truncated or rotated, we reset lastSizeSeen
			c.lastSizeSeen = 0
			console.Close()

			if console, err = os.OpenFile(c.SyslogFile, os.O_RDONLY|unix.O_NONBLOCK, 0666); err != nil {
				glog.Errorf("Error opening syslog file %s: %v", c.SyslogFile, err)

				return false, nil, err
			}
		}

		return true, console, nil
	}

	return false, console, nil
}

func (c *Console) copyBuffer(ctx context.Context, dst *os.File) (written int64, err error) {
	var console *os.File = nil

	buf := make([]byte, 32*1024)
	changed := false
	errInvalidWrite := errors.New("invalid write result")

	close := func() {
		if console != nil {
			console.Close()
		}
	}

	defer close()

	for err == nil {
		select {
		case <-ctx.Done():
			glog.Debug("Console forwarding stopped by context")
			return written, nil
		default:
			if changed, console, err = c.syslogChanged(console); changed {
				var readfd unix.FdSet
				var timeout = unix.Timeval{
					Sec:  0,
					Usec: 1000,
				}

				readfd.Zero()
				readfd.Set(int(console.Fd()))

				glog.Debugf("Waiting for data on console syslog file %s", c.SyslogFile)

				if _, err = unix.Select(int(console.Fd())+1, &readfd, nil, nil, &timeout); err != nil {
					if utils.IsEINTR(err) {
						continue
					} else {
						glog.Errorf("Error selecting %s: %v", c.SyslogFile, err)
						return
					}
				}

				if readfd.IsSet(int(console.Fd())) {
					glog.Debugf("Reading from console syslog file %s", c.SyslogFile)

					var available int

					for {
						if available, err = console.Read(buf); available > 0 {
							glog.Debugf("Read %d bytes from console syslog file %s", available, c.SyslogFile)

							c.lastSizeSeen, _ = console.Seek(0, io.SeekCurrent)

							nw, ew := dst.Write(buf[0:available])

							if nw < 0 || available < nw {
								nw = 0
								if ew == nil {
									ew = errInvalidWrite
								}
							}

							written += int64(nw)

							if ew != nil {
								err = ew
								break
							}

							if available != nw {
								err = io.ErrShortWrite
								break
							}
						} else if err == io.EOF {
							glog.Debugf("No more data to read from console syslog file %s", c.SyslogFile)
							err = nil
							break
						}
					}
				}
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	glog.Debugf("Console forwarding stopped: %v, total bytes written: %d", err, written)

	return written, err
}
