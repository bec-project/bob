package containermanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
