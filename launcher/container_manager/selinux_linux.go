package containermanager

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

const selinuxXattr = "security.selinux"

// getSELinuxLabels returns the sandbox's labels where SELinux is on, after checking that the
// policy knows container_t.
func getSELinuxLabels() (selinuxLabels, error) {
	if _, err := os.Stat("/sys/fs/selinux/enforce"); err != nil {
		return selinuxLabels{}, nil
	}
	labels := newSELinuxLabels(os.Getuid())
	// The kernel checks a context written to this file and refuses one it does not know.
	check, err := os.OpenFile("/sys/fs/selinux/context", os.O_RDWR, 0)
	if err != nil {
		return selinuxLabels{}, fmt.Errorf("check SELinux labels: %w", err)
	}
	defer check.Close()
	if _, err := check.Write([]byte(labels.process)); err != nil {
		return selinuxLabels{}, fmt.Errorf("SELinux is on but its policy has no container_t; install container-selinux: %w", err)
	}
	return labels, nil
}

// relabelTree gives path and everything under it the sandbox's file type and level, keeping each
// file's SELinux user and role. A file of another user is an error: the sandbox could not use it.
func (l selinuxLabels) relabelTree(path string) error {
	if !l.on() {
		return nil
	}
	return filepath.WalkDir(path, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Symbolic links keep their labels; the files they point to are labelled where they are.
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !ownedByUser(info) {
			return fmt.Errorf("%s belongs to another user; the sandbox cannot use it", name)
		}
		return l.relabel(name)
	})
}

// relabelIfOwned labels one file of this user for the sandbox; system files keep their labels,
// which container_t may read.
func (l selinuxLabels) relabelIfOwned(path string) error {
	if !l.on() {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !ownedByUser(info) {
		return nil
	}
	return l.relabel(path)
}

func (l selinuxLabels) relabel(name string) error {
	current := make([]byte, 256)
	n, err := syscall.Getxattr(name, selinuxXattr, current)
	if err != nil {
		return fmt.Errorf("read the SELinux label of %s: %w", name, err)
	}
	have := strings.TrimRight(string(current[:n]), "\x00")
	wanted := withTypeAndLevel(have, "container_file_t", l.level)
	if wanted == have {
		return nil
	}
	if err := syscall.Setxattr(name, selinuxXattr, []byte(wanted), 0); err != nil {
		return fmt.Errorf("label %s for the sandbox: %w", name, err)
	}
	return nil
}

func ownedByUser(info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

// listenUnix runs listen with the sandbox's label for the sockets it creates, so the sandbox may
// connect to them.
func (l selinuxLabels) listenUnix(listen func() error) error {
	if !l.on() {
		return listen()
	}
	// The label applies to sockets this thread creates, until it is cleared.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const attribute = "/proc/thread-self/attr/sockcreate"
	if err := os.WriteFile(attribute, []byte(l.process), 0); err != nil {
		return fmt.Errorf("label bob-manager's sockets for the sandbox: %w", err)
	}
	listenErr := listen()
	if err := os.WriteFile(attribute, nil, 0); err != nil {
		return errors.Join(listenErr, err)
	}
	return listenErr
}
