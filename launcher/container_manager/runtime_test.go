package containermanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandSeparatesWarningsAndPreservesLogStreams(t *testing.T) {
	output, err := command(context.Background(), "sh", nil, "-c", "printf container-id; printf warning >&2")
	if err != nil || output != "container-id" {
		t.Fatalf("warning corrupted structured output: %q, %v", output, err)
	}
	output, err = commandLogs(context.Background(), "sh", "-c", "printf stdout-log; printf stderr-log >&2")
	if err != nil || !strings.Contains(output, "stdout-log") || !strings.Contains(output, "stderr-log") {
		t.Fatalf("logs lost a stream: %q, %v", output, err)
	}
	_, err = command(context.Background(), "sh", nil, "-c", "printf stdout-error; printf stderr-error >&2; exit 1")
	if err == nil || !strings.Contains(err.Error(), "stdout-error") || !strings.Contains(err.Error(), "stderr-error") {
		t.Fatalf("failure lost diagnostic output: %v", err)
	}
}

func TestCreationFailureDistinguishesRejectionFromLostAcknowledgement(t *testing.T) {
	for _, test := range []struct {
		name, message   string
		cancel, settled bool
	}{
		{"image rejection", "Error response from daemon: No such image: missing", false, true},
		{"prefixed rejection", "docker: Error response from daemon: invalid mount config for type bind", false, true},
		{"transport failure", "connection reset by peer", false, false},
		{"proxy timeout", "Error response from daemon: <html>504 Gateway Timeout</html>", false, false},
		{"daemon timeout", "Error response from daemon: context deadline exceeded", false, false},
		{"unknown failure", "Error response from daemon: internal server error", false, false},
		{"cancelled response", "Error response from daemon: No such image: missing", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "docker")
			script := "#!/bin/sh\nprintf '%s\\n' \"$BOB_TEST_ERROR\" >&2\n"
			if test.cancel {
				script += "sleep 30\n"
			}
			script += "exit 1\n"
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if test.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			_, err := command(ctx, binary, []string{"BOB_TEST_ERROR=" + test.message}, "create")
			var failure *commandFailure
			if !errors.As(err, &failure) || failure.creationSettled != test.settled {
				t.Fatalf("wrong creation outcome classification: %v, %#v", err, failure)
			}
		})
	}
	_, err := command(context.Background(), filepath.Join(t.TempDir(), "missing"), nil, "create")
	var failure *commandFailure
	if !errors.As(err, &failure) || !failure.creationSettled {
		t.Fatal("failure before process start was treated as dispatched creation")
	}
}

func TestCommandCancellationStopsDescendants(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "orphan")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// Killing only the parent shell leaves the background process alive to write
	// this file, and its inherited output pipe keeps CombinedOutput waiting.
	_, err := command(ctx, "sh", nil, "-c", `(sleep 0.5; touch "$1") & wait`, "test", marker)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation returned %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a command descendant survived cancellation")
	}
}
