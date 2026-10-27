package containermanager

import (
	"fmt"
	"strings"
)

// Where SELinux is on, the sandbox runs as container_t, the type rootless containers run as, with
// two categories of the user's own. It can then touch only files labelled container_file_t with
// those categories: the workspace mounts, bob's home and run directory, which bob-manager labels
// before each start (as podman's :Z did), and the host's /usr and /etc, which container_t may
// read. The listening sockets bob-manager makes for the sandbox carry its label, since a
// container_t process may only connect to container_t sockets.

// selinuxLabels is the sandbox's process and file label, or empty when SELinux is off.
type selinuxLabels struct {
	process string
	file    string
	level   string
}

func (l selinuxLabels) on() bool { return l.process != "" }

func newSELinuxLabels(uid int) selinuxLabels {
	level := selinuxLevel(uid)
	return selinuxLabels{
		process: "system_u:system_r:container_t:" + level,
		file:    "system_u:object_r:container_file_t:" + level,
		level:   level,
	}
}

// selinuxLevel gives each user two categories of their own, the same on every start, so their
// files keep their labels between starts and another user's sandbox cannot use them. The pairs
// of the 1024 categories number 523776; UIDs that far apart share one.
func selinuxLevel(uid int) string {
	n := uid % (1024 * 1023 / 2)
	first := 0
	for n >= 1023-first {
		n -= 1023 - first
		first++
	}
	return fmt.Sprintf("s0:c%d,c%d", first, first+1+n)
}

// withTypeAndLevel replaces the type and level of an SELinux context.
func withTypeAndLevel(context, kind, level string) string {
	parts := strings.SplitN(context, ":", 4)
	if len(parts) < 3 {
		return "system_u:object_r:" + kind + ":" + level
	}
	return parts[0] + ":" + parts[1] + ":" + kind + ":" + level
}
