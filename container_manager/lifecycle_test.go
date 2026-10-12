package containermanager

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The fake engine models a daemon accepting create before its CLI exits. The
// ownership filter is checked so these tests cannot pass by deleting by name.
func cancellationEngine(t *testing.T) (Options, string) {
	t.Helper()
	root := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$BOB_TEST_STATE/commands"
case "$1" in
  info) printf '%s\n' "${BOB_TEST_SECURITY_OPTIONS:-[]}" ;;
  image) if [ -e "$BOB_TEST_STATE/image-error" ]; then echo 'image missing' >&2; exit 1; fi ;;
  create)
    if [ -e "$BOB_TEST_STATE/conflict" ]; then exit 1; fi
    helper=0
    label=
    while [ "$#" -gt 0 ]; do
      if [ "$1" = --label ]; then label="$2"; fi
      if [ "$1" = --entrypoint ] && [ "$2" = chown ]; then helper=1; fi
      shift
    done
    if [ "$helper" = 1 ]; then
      if [ -e "$BOB_TEST_STATE/reject-home" ]; then
        echo 'Error response from daemon: No such image: missing-helper' >&2
        exit 1
      fi
      printf '%s' "$label" > "$BOB_TEST_STATE/home-label"
      touch "$BOB_TEST_STATE/home-created"
      echo synthetic-home-id
      if [ -e "$BOB_TEST_STATE/cancel-home-create" ]; then sleep 30; fi
      exit 0
    fi
    printf '%s' "$label" > "$BOB_TEST_STATE/label"
    if [ -e "$BOB_TEST_STATE/reject-create" ]; then
      echo 'Error response from daemon: No such image: missing-agent' >&2
      exit 1
    fi
    if [ -e "$BOB_TEST_STATE/transport-error" ]; then
      touch "$BOB_TEST_STATE/accepted"
      echo 'connection reset by peer' >&2
      exit 1
    fi
    if [ -e "$BOB_TEST_STATE/delay-create" ]; then
      touch "$BOB_TEST_STATE/accepted"
      sleep 30
      exit 0
    fi
    touch "$BOB_TEST_STATE/created"
    printf 'synthetic-container-id\n'
    if [ -e "$BOB_TEST_STATE/quick-create" ]; then exit 0; fi
    sleep 30 ;;
  ps)
    if [ "$4" = name=fixed-name ]; then
      if [ -e "$BOB_TEST_STATE/conflict" ]; then echo fixed-name; fi
      exit 0
    fi
    if [ -e "$BOB_TEST_STATE/home-label" ] && [ "$4" = "label=$(cat "$BOB_TEST_STATE/home-label")" ]; then
      if [ -e "$BOB_TEST_STATE/home-created" ]; then echo synthetic-home-id; fi
      exit 0
    fi
    touch "$BOB_TEST_STATE/looked-up"
    if [ -e "$BOB_TEST_STATE/fail-lookup" ]; then exit 1; fi
    if [ -e "$BOB_TEST_STATE/created" ]; then
      test "$4" = "label=$(cat "$BOB_TEST_STATE/label")" || exit 2
      printf 'synthetic-container-id\n'
    fi ;;
  rm)
    if [ "$3" = synthetic-home-id ]; then
      rm "$BOB_TEST_STATE/home-created" || exit 1
      touch "$BOB_TEST_STATE/home-removed"
      exit 0
    fi
    test "$3" = synthetic-container-id || exit 2
    if [ -e "$BOB_TEST_STATE/fail-remove" ]; then exit 1; fi
    if [ ! -e "$BOB_TEST_STATE/created" ]; then echo 'No such container' >&2; exit 1; fi
    rm "$BOB_TEST_STATE/created"
    touch "$BOB_TEST_STATE/removed"
    if [ -e "$BOB_TEST_STATE/lose-remove-response" ]; then exit 1; fi ;;
  inspect) if [ -e "$BOB_TEST_STATE/home-failed" ]; then echo 42; else echo 0; fi ;;
esac
`
	if err := os.WriteFile(filepath.Join(root, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BOB_TEST_STATE", root)
	options := DefaultOptions()
	options.Runtime, options.Name, options.Workdir = "docker", "fixed-name", root
	options.BaseURL = "https://model.example/agent/"
	return options, root
}

func cancelAfterCreate(t *testing.T, root string, cancel context.CancelFunc) <-chan struct{} {
	return cancelAfterMarker(t, root, "created", cancel)
}

func cancelAfterMarker(t *testing.T, root, marker string, cancel context.CancelFunc) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	return done
}

func TestCancelledCreateRemovesOwnedContainer(t *testing.T) {
	options, root := cancellationEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := cancelAfterCreate(t, root, cancel)
	manager := &Manager{}
	_, err := manager.Start(ctx, options)
	<-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "removed")); err != nil {
		t.Fatalf("accepted container was not removed: %v", err)
	}
	if manager.name != "" || manager.launchID != "" {
		t.Fatal("successful cleanup retained launch state")
	}
}

func TestUncertainCreateRetainsOwnershipForCleanupRetry(t *testing.T) {
	options, root := cancellationEngine(t)
	if err := os.WriteFile(filepath.Join(root, "fail-lookup"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := cancelAfterCreate(t, root, cancel)
	manager := &Manager{}
	_, err := manager.Start(ctx, options)
	<-done
	if err == nil || !strings.Contains(err.Error(), "startup cleanup") || manager.launchID == "" {
		t.Fatalf("failed cleanup lost ownership: %v, %q", err, manager.launchID)
	}
	if _, err := manager.Start(context.Background(), options); err == nil {
		t.Fatal("allowed restart before pending cleanup")
	}
	if err := os.Remove(filepath.Join(root, "fail-lookup")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "removed")); err != nil {
		t.Fatal("cleanup retry did not remove the owned container")
	}
}

func TestConflictingCreateNeverRemovesExistingContainer(t *testing.T) {
	options, root := cancellationEngine(t)
	if err := os.WriteFile(filepath.Join(root, "conflict"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{}
	if _, err := manager.Start(context.Background(), options); err == nil {
		t.Fatal("accepted conflicting creation")
	}
	commands, err := os.ReadFile(filepath.Join(root, "commands"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(commands), "rm --force") {
		t.Fatalf("attempted to remove an unrelated container: %s", commands)
	}
	if manager.launchID != "" {
		t.Fatal("retained ownership after confirming no container was created")
	}
}

func TestCancelledCreateWaitsForDelayedContainer(t *testing.T) {
	options, root := cancellationEngine(t)
	if err := os.WriteFile(filepath.Join(root, "delay-create"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The daemon is independent of the cancelled CLI. Commit creation only
	// after cleanup has already queried ownership and found nothing.
	done := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for _, marker := range []string{"accepted", "looked-up"} {
			for {
				if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
					break
				}
				if time.Now().After(deadline) {
					cancel()
					done <- errors.New("timed out waiting for " + marker)
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			cancel()
		}
		time.Sleep(100 * time.Millisecond)
		done <- os.WriteFile(filepath.Join(root, "created"), nil, 0600)
	}()
	manager := &Manager{}
	_, err := manager.Start(ctx, options)
	if daemonErr := <-done; daemonErr != nil {
		t.Fatal(daemonErr)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "removed")); err != nil {
		t.Fatalf("late-created container was not removed: %v", err)
	}
	if manager.launchID != "" {
		t.Fatal("successful cleanup retained ownership")
	}
}

func TestStopRecoversLostRemovalResponse(t *testing.T) {
	_, root := cancellationEngine(t)
	for _, marker := range []string{"created", "lose-remove-response"} {
		if err := os.WriteFile(filepath.Join(root, marker), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "label"), []byte(launchLabel+"=owned"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{binary: filepath.Join(root, "docker"), name: "synthetic-container-id", launchID: "owned"}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatalf("removal succeeded but lost response blocked cleanup: %v", err)
	}
	if manager.name != "" || manager.launchID != "" || manager.binary != "" {
		t.Fatal("confirmed removal retained launch state")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStopRetainsPendingCreationAfterEmptyLookup(t *testing.T) {
	options, root := cancellationEngine(t)
	if err := os.WriteFile(filepath.Join(root, "label"), []byte(launchLabel+"=owned"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{binary: filepath.Join(root, "docker"), launchID: "owned", createUncertain: true}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := manager.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unresolved creation should report timeout: %v", err)
	}
	if manager.launchID != "owned" || !manager.createUncertain {
		t.Fatal("empty lookup discarded uncertain ownership")
	}
	if _, err := manager.Start(context.Background(), options); err == nil {
		t.Fatal("allowed restart while creation was unresolved")
	}
	if err := os.WriteFile(filepath.Join(root, "created"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.launchID != "" || manager.createUncertain {
		t.Fatal("cleanup retry retained settled ownership")
	}
}

func TestStopRetainsStateUntilRemovalIsConfirmed(t *testing.T) {
	for _, failure := range []string{"fail-remove", "fail-lookup"} {
		t.Run(failure, func(t *testing.T) {
			_, root := cancellationEngine(t)
			for _, marker := range []string{"created", "lose-remove-response", failure} {
				if err := os.WriteFile(filepath.Join(root, marker), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "label"), []byte(launchLabel+"=owned"), 0600); err != nil {
				t.Fatal(err)
			}
			manager := &Manager{binary: filepath.Join(root, "docker"), name: "synthetic-container-id", launchID: "owned"}
			if err := manager.Stop(context.Background()); err == nil {
				t.Fatal("cleanup succeeded without confirming removal")
			}
			if manager.name == "" || manager.launchID != "owned" {
				t.Fatal("failed cleanup discarded ownership")
			}
			if err := os.Remove(filepath.Join(root, failure)); err != nil {
				t.Fatal(err)
			}
			if err := manager.Stop(context.Background()); err != nil {
				t.Fatalf("cleanup retry failed: %v", err)
			}
		})
	}
}

func TestTransportFailureRetainsCreationOwnership(t *testing.T) {
	options, root := cancellationEngine(t)
	if err := os.WriteFile(filepath.Join(root, "transport-error"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{}
	resolved := resolvedOptions{Options: options, launchID: "owned"}
	args, env, err := getLaunchArgs(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.createContainer(context.Background(), filepath.Join(root, "docker"), "owned", env, args...); err == nil {
		t.Fatal("expected transport failure")
	}
	if !manager.createUncertain || manager.launchID != "owned" {
		t.Fatal("transport failure discarded ownership")
	}
	// An empty snapshot cannot prove the failed request will never complete.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := manager.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected unresolved cleanup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "created"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "removed")); err != nil {
		t.Fatal("late container survived cleanup retry")
	}
}

func TestHomePreparationCancellationIsTracked(t *testing.T) {
	options, root := cancellationEngine(t)
	if err := os.WriteFile(filepath.Join(root, "cancel-home-create"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedOptions{Options: options, binary: filepath.Join(root, "docker"), launchID: "owned", userConfig: userSettings{User: "1000:1000"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := cancelAfterMarker(t, root, "home-created", cancel)
	manager := &Manager{}
	err := manager.prepareHome(ctx, resolved)
	<-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected helper cancellation: %v", err)
	}
	if manager.home == nil || manager.home.launchID != "owned-home" {
		t.Fatal("cancelled helper lost ownership")
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if manager.home != nil {
		t.Fatal("successful helper cleanup retained state")
	}
	if _, err := os.Stat(filepath.Join(root, "home-removed")); err != nil {
		t.Fatal("helper survived cleanup")
	}
}

func TestHomePreparationChecksExitCodeAndRemovesHelper(t *testing.T) {
	options, root := cancellationEngine(t)
	if err := os.WriteFile(filepath.Join(root, "quick-create"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedOptions{Options: options, binary: filepath.Join(root, "docker"), launchID: "owned", userConfig: userSettings{User: "1000:1000"}}
	manager := &Manager{}
	if err := manager.prepareHome(context.Background(), resolved); err != nil {
		t.Fatal(err)
	}
	if manager.home != nil {
		t.Fatal("completed helper retained state")
	}
	commands, err := os.ReadFile(filepath.Join(root, "commands"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(commands), "inspect --format {{.State.ExitCode}}") {
		t.Fatal("helper exit status was not checked")
	}
	if _, err := os.Stat(filepath.Join(root, "home-removed")); err != nil {
		t.Fatal("completed helper was not removed")
	}
}

func TestDefiniteCreateRejectionAllowsRetry(t *testing.T) {
	options, root := cancellationEngine(t)
	if err := os.WriteFile(filepath.Join(root, "reject-create"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{}
	if _, err := manager.Start(context.Background(), options); err == nil || !strings.Contains(err.Error(), "No such image") {
		t.Fatalf("expected engine rejection: %v", err)
	}
	if manager.launchID != "" || manager.home != nil {
		t.Fatal("definite rejection retained ownership")
	}
	if err := os.Remove(filepath.Join(root, "reject-create")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "quick-create"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	resolved := resolvedOptions{Options: options, launchID: "retry"}
	args, env, err := getLaunchArgs(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.createContainer(context.Background(), filepath.Join(root, "docker"), "retry", env, args...); err != nil {
		t.Fatalf("corrected retry failed: %v", err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHomePreparationFailureIsReportedAndCleaned(t *testing.T) {
	for _, marker := range []string{"reject-home", "home-failed"} {
		t.Run(marker, func(t *testing.T) {
			options, root := cancellationEngine(t)
			if err := os.WriteFile(filepath.Join(root, marker), nil, 0600); err != nil {
				t.Fatal(err)
			}
			resolved := resolvedOptions{Options: options, binary: filepath.Join(root, "docker"), launchID: "owned", userConfig: userSettings{User: "1000:1000"}}
			manager := &Manager{}
			if err := manager.prepareHome(context.Background(), resolved); err == nil {
				t.Fatal("failed helper was accepted")
			}
			if err := manager.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			if manager.home != nil {
				t.Fatal("helper failure retained ownership after cleanup")
			}
		})
	}
}

func TestImageFailureAndUnsupportedMappingFailBeforeCreation(t *testing.T) {
	for _, failure := range []string{"image", "rootless", "userns"} {
		t.Run(failure, func(t *testing.T) {
			options, root := cancellationEngine(t)
			if failure == "image" {
				if err := os.WriteFile(filepath.Join(root, "image-error"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv("BOB_TEST_SECURITY_OPTIONS", `["name=`+failure+`"]`)
			}
			manager := &Manager{}
			if _, err := manager.Start(context.Background(), options); err == nil {
				t.Fatal("invalid engine/image was accepted")
			}
			commands, err := os.ReadFile(filepath.Join(root, "commands"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(commands), "create ") || manager.launchID != "" || manager.home != nil {
				t.Fatal("preflight failure dispatched creation or retained state")
			}
		})
	}
}

func TestContainerCancelledCreateIntegration(t *testing.T) {
	if os.Getenv("BOB_CONTAINER_TEST") != "1" {
		t.Skip("set BOB_CONTAINER_TEST=1 to run against Docker")
	}
	binary, err := getExecutable("docker")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// Let the real daemon create the container, then hold the CLI open so
	// cancellation occurs after acceptance but before Start receives its ID.
	script := `#!/bin/sh
case "$*" in
  *"--entrypoint chown"*) exec "$BOB_TEST_DOCKER" "$@" ;;
esac
if [ "$1" = create ]; then
  "$BOB_TEST_DOCKER" "$@" || exit "$?"
  touch "$BOB_TEST_STATE/created"
  sleep 30
else
  exec "$BOB_TEST_DOCKER" "$@"
fi
`
	if err := os.WriteFile(filepath.Join(root, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BOB_TEST_DOCKER", binary)
	t.Setenv("BOB_TEST_STATE", root)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	options := DefaultOptions()
	options.Runtime, options.Workdir = "docker", root
	options.BaseURL = "https://model.example/agent/"
	options.Name = "bob-create-cancel-" + filepath.Base(root) + "-" + strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	options.HomeVolume = options.Name + "-home"
	manager := &Manager{}
	t.Cleanup(func() {
		// Limit fallback cleanup to the unique test name, even if Start failed.
		_ = manager.Stop(context.Background())
		_ = exec.Command(binary, "rm", "--force", options.Name).Run()
		if output, err := exec.Command(binary, "volume", "rm", options.HomeVolume).CombinedOutput(); err != nil {
			t.Errorf("remove test volume: %s, %v", output, err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := cancelAfterCreate(t, root, cancel)
	_, err = manager.Start(ctx, options)
	<-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation after creation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "created")); err != nil {
		t.Fatalf("real engine never accepted create: %v", err)
	}
	output, err := exec.Command(binary, "ps", "--all", "--filter", "name="+options.Name, "--format", "{{.ID}}").Output()
	if err != nil || strings.TrimSpace(string(output)) != "" {
		t.Fatalf("cancelled creation left a container: %q, %v", output, err)
	}
	if manager.name != "" || manager.launchID != "" {
		t.Fatal("successful cleanup retained launch state")
	}
}
