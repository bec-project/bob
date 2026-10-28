package sandbox

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSandboxHelperRefusesAnUnconfinedCommand(t *testing.T) {
	if os.Getenv("BOB_TEST_UNCONFINED_HELPER") == "1" {
		if err := RunSandboxHelper([]string{os.Args[0], "-test.run=^TestConfinementUnexpectedCommand$"}); err != nil {
			fmt.Println(err)
			os.Exit(0)
		}
		os.Exit(1)
	}
	directory, err := os.MkdirTemp("/tmp", "bob-confinement-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	listener, err := net.Listen("unix", filepath.Join(directory, "way-in.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	file, err := listener.(*net.UnixListener).File()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	marker := filepath.Join(directory, "OpenCode-started")
	child := exec.Command(os.Args[0], "-test.run=^TestSandboxHelperRefusesAnUnconfinedCommand$")
	child.Stdin = file
	child.Env = append(os.Environ(), "BOB_TEST_UNCONFINED_HELPER=1", "BOB_TEST_CONFINEMENT_MARKER="+marker)
	output, err := child.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "refusing to start OpenCode") {
		t.Fatalf("unconfined helper = %v: %s", err, output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("an unconfined helper started its command: %v", err)
	}
}

func TestConfinementProbeRejectsAnOrdinaryProcess(t *testing.T) {
	if os.Getenv("BOB_TEST_CONFINEMENT_PROBE") == "1" {
		if err := runConfinementProbe(); err != nil {
			fmt.Println(err)
			os.Exit(0)
		}
		os.Exit(1)
	}
	// Establish that this test process can create a Unix socket before asking the disposable
	// child to prove the opposite. Namespace-changing probes never run in the test process.
	directory, err := os.MkdirTemp("/tmp", "bob-probe-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	listener, err := net.Listen("unix", filepath.Join(directory, "socket"))
	if err != nil {
		t.Skipf("an outer sandbox already refuses Unix sockets: %v", err)
	}
	listener.Close()
	child := exec.Command(os.Args[0], "-test.run=^TestConfinementProbeRejectsAnOrdinaryProcess$")
	child.Env = append(os.Environ(), "BOB_TEST_CONFINEMENT_PROBE=1")
	output, err := child.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "socket(AF_UNIX)") {
		t.Fatalf("probe of an ordinary process = %v: %s", err, output)
	}
}

func TestConfinementUnexpectedCommand(t *testing.T) {
	if marker := os.Getenv("BOB_TEST_CONFINEMENT_MARKER"); marker != "" {
		if err := os.WriteFile(marker, []byte("started"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
