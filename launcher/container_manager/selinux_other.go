//go:build !linux

package containermanager

// SELinux exists on Linux only.
func getSELinuxLabels() (selinuxLabels, error)             { return selinuxLabels{}, nil }
func (selinuxLabels) relabelTree(string) error             { return nil }
func (selinuxLabels) relabelIfOwned(string) error          { return nil }
func (selinuxLabels) listenUnix(listen func() error) error { return listen() }
