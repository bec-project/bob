package main

import (
	"bytes"
	"encoding/binary"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// Run the actual broker in a separate process group, so SIGINT models an interrupt from outside.
func TestSignalHelper(t *testing.T) {
	if os.Getenv("BOB_SIGNAL_HELPER") != "1" {
		return
	}
	os.Args = []string{"bob-manager", "--broker", "--config", os.Getenv("BOB_SIGNAL_CONFIG")}
	if err := run(); err != nil {
		os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func TestCtrlCDuringStartupAndCleanup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the Ctrl-C test runs on Linux only")
	}
	directory := t.TempDir()
	// A stand-in Node.js (which runs sandbox-runtime) that never becomes ready, notes the stop it
	// is asked for, and takes a while to finish it. The sandbox gets no variable of ours, so the
	// script names its folder itself.
	script := `#!/bin/sh
if [ "$1" = --version ]; then printf 'v22.12.0\n'; exit 0; fi
trap '/usr/bin/touch "` + directory + `/interrupted"; exit 130' INT
trap '/usr/bin/touch "` + directory + `/down"; /usr/bin/sleep 0.5; /usr/bin/touch "` + directory + `/cleaned"; exit 0' TERM
/usr/bin/touch "` + directory + `/up"
while :; do /usr/bin/sleep 0.1; done
`
	if err := os.WriteFile(filepath.Join(directory, "node"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	openCode := filepath.Join(directory, "opencode")
	for _, tool := range []string{"bwrap", "socat", "rg", "opencode"} {
		if err := os.WriteFile(filepath.Join(directory, tool), []byte("#!/bin/sh\nif [ \"$1\" = --version ]; then printf '1.18.35\\n'; exit 0; fi\nexit 1\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	srt := filepath.Join(directory, "srt", "dist", "cli.js")
	if err := os.MkdirAll(filepath.Dir(srt), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srt, nil, 0600); err != nil {
		t.Fatal(err)
	}
	architecture, machine := "x64", uint16(62)
	if runtime.GOARCH == "arm64" {
		architecture, machine = "arm64", 183
	}
	seccomp := filepath.Join(directory, "srt", "vendor", "seccomp", architecture, "apply-seccomp")
	if err := os.MkdirAll(filepath.Dir(seccomp), 0o700); err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 64)
	copy(header, "\x7fELF")
	header[4], header[5], header[6] = 2, 1, 1
	binary.LittleEndian.PutUint16(header[16:], 2)
	binary.LittleEndian.PutUint16(header[18:], machine)
	binary.LittleEndian.PutUint32(header[20:], 1)
	binary.LittleEndian.PutUint16(header[52:], 64)
	if err := os.WriteFile(seccomp, header, 0o700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(directory, "bob.toml")
	content := "[sandbox]\nopencode = \"" + openCode + "\"\nruntime = \"" + srt + "\"\nhome = \"home\"\n[provider]\nbase_url = \"http://model.example/api/\"\n"
	if err := os.WriteFile(config, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	// Unix socket paths are short; t.TempDir can exceed the limit on macOS.
	brokerDir, err := os.MkdirTemp("", "bob")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(brokerDir) })
	cmd := exec.Command(os.Args[0], "-test.run=^TestSignalHelper$")
	cmd.Env = append(os.Environ(), "BOB_SIGNAL_HELPER=1", "BOB_SIGNAL_CONFIG="+config,
		"BOB_RUNTIME_DIR="+brokerDir, "XDG_RUNTIME_DIR="+brokerDir, "PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	finished := false
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		if !finished {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	})
	// bob starts for its first client, which holds the lease through its connection.
	socket := filepath.Join(brokerDir, brokerSocket)
	waitForFile(t, socket)
	client, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Write([]byte(`{"consumer":"test","pid":1,"config":"bob.toml"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, filepath.Join(directory, "up"))
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	waitForFile(t, filepath.Join(directory, "down"))
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatalf("Ctrl-C failed: %v\n%s", err, output.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Ctrl-C did not finish cleanup")
	}
	if _, err := os.Stat(filepath.Join(directory, "interrupted")); !os.IsNotExist(err) {
		t.Fatal("terminal Ctrl-C reached the sandbox")
	}
	if _, err := os.Stat(filepath.Join(directory, "cleaned")); err != nil {
		t.Fatal("the sandbox did not finish its stop")
	}
}
