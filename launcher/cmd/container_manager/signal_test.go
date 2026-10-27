package main

import (
	"bytes"
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
		t.Skip("bob's sandbox runs on Linux only")
	}
	directory := t.TempDir()
	// A stand-in bwrap that never becomes ready, notes the stop it is asked for, and takes a while
	// to finish it. The sandbox gets no variable of ours, so the script names its folder itself.
	script := `#!/bin/sh
trap 'touch "` + directory + `/interrupted"; exit 130' INT
trap 'touch "` + directory + `/down"; sleep 0.5; touch "` + directory + `/cleaned"; exit 0' TERM
touch "` + directory + `/up"
while :; do sleep 0.1; done
`
	if err := os.WriteFile(filepath.Join(directory, "bwrap"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	openCode := filepath.Join(directory, "opencode")
	if err := os.WriteFile(openCode, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(directory, "bob.toml")
	content := "[sandbox]\nopencode = \"" + openCode + "\"\nhome = \"home\"\n[workspace]\nworkdir = \".\"\n[provider]\nbase_url = \"http://model.example/api/\"\n"
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
