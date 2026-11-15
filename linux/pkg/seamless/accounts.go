package seamless

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Account is a line of /etc/passwd.
type Account struct {
	Name  string
	UID   uint32
	GID   uint32
	Home  string
	Shell string
}

// ParsePasswd reads /etc/passwd format, ignoring comments and malformed lines.
func ParsePasswd(r io.Reader) []Account {
	var accounts []Account

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Split(line, ":")

		if len(fields) < 7 {
			continue
		}

		uid, err := strconv.ParseUint(fields[2], 10, 32)

		if err != nil {
			continue
		}

		gid, err := strconv.ParseUint(fields[3], 10, 32)

		if err != nil {
			continue
		}

		accounts = append(accounts, Account{Name: fields[0], UID: uint32(uid), GID: uint32(gid), Home: fields[5], Shell: fields[6]})
	}

	return accounts
}

func canLogin(a Account) bool {
	switch a.Shell {
	case "", "/usr/sbin/nologin", "/sbin/nologin", "/bin/false", "/usr/bin/false":
		return false
	}

	return true
}

// PickAccount returns the named account, or, when name is empty, the regular user with the lowest uid: a login
// shell and a uid in the range distributions give to people (1000...59999).
func PickAccount(accounts []Account, name string) (Account, error) {
	if name != "" {
		for _, a := range accounts {
			if a.Name == name {
				return a, nil
			}
		}

		return Account{}, fmt.Errorf("user %q does not exist", name)
	}

	var best *Account

	for i := range accounts {
		a := accounts[i]

		if a.UID < 1000 || a.UID >= 60000 || !canLogin(a) {
			continue
		}

		if best == nil || a.UID < best.UID {
			best = &a
		}
	}

	if best == nil {
		return Account{}, fmt.Errorf("no regular user found")
	}

	return *best, nil
}
