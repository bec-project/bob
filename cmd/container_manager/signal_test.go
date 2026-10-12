package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// Run the actual CLI in a separate process group, so SIGINT models terminal Ctrl-C.
func TestSignalHelper(t *testing.T) {
	if os.Getenv("BOB_SIGNAL_HELPER") != "1" {
		return
	}
	os.Args = []string{"bob-manager", "--config", os.Getenv("BOB_SIGNAL_CONFIG")}
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
	testCtrlC(t, false)
}

func TestCtrlCReportsCleanupFailure(t *testing.T) {
	testCtrlC(t, true)
}

func testCtrlC(t *testing.T, failCleanup bool) {
	t.Helper()
	directory := t.TempDir()
	engine := filepath.Join(directory, "docker")
	// Model runtime subprocesses and a cleanup operation long enough to interrupt.
	script := `#!/bin/sh
case "$1" in
    info) echo '[]'; exit 0 ;;
    create) case "$*" in *"--entrypoint chown"*) echo synthetic-home-id; exit 0 ;; esac; touch "$BOB_SIGNAL_STATE/created"; echo synthetic-container-id; exit 0 ;;
    start) if [ "$2" = --attach ]; then exit 0; fi; touch "$BOB_SIGNAL_STATE/up"; sleep 30; exit 0 ;;
    inspect) echo 0; exit 0 ;;
    ps) if [ -e "$BOB_SIGNAL_STATE/created" ]; then echo synthetic-container-id; fi; exit 0 ;;
    rm) if [ "$3" = synthetic-home-id ]; then exit 0; fi; trap 'touch "$BOB_SIGNAL_STATE/interrupted"; exit 130' INT; touch "$BOB_SIGNAL_STATE/down"; sleep 0.5; if [ "$BOB_SIGNAL_FAIL_CLEANUP" = 1 ]; then echo "synthetic cleanup failure" >&2; exit 1; fi; rm "$BOB_SIGNAL_STATE/created"; touch "$BOB_SIGNAL_STATE/cleaned"; exit 0 ;;
esac
exit 0
`
	if err := os.WriteFile(engine, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(directory, "bob.toml")
	content := "[container]\nruntime = \"docker\"\n[workspace]\nworkdir = \".\"\n[provider]\nbase_url = \"http://127.0.0.1:12345/api/\"\n"
	if err := os.WriteFile(config, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSignalHelper$")
	fail := "0"
	if failCleanup {
		fail = "1"
	}
	cmd.Env = append(os.Environ(), "BOB_SIGNAL_HELPER=1", "BOB_SIGNAL_CONFIG="+config,
		"BOB_SIGNAL_STATE="+directory, "BOB_SIGNAL_FAIL_CLEANUP="+fail, "PATH="+directory+string(os.PathListSeparator)+os.Getenv("PATH"))
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
		if failCleanup {
			if err == nil || !bytes.Contains(output.Bytes(), []byte("cleanup:")) {
				t.Fatalf("cleanup failure was not reported: %v %s", err, output.String())
			}
			return
		}
		if err != nil {
			t.Fatalf("Ctrl-C failed: %v\n%s", err, output.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Ctrl-C did not finish cleanup")
	}
	if _, err := os.Stat(filepath.Join(directory, "interrupted")); !os.IsNotExist(err) {
		t.Fatal("terminal Ctrl-C interrupted the cleanup command")
	}
	if _, err := os.Stat(filepath.Join(directory, "cleaned")); err != nil {
		t.Fatal("cleanup did not complete")
	}
}
