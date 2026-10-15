// container_manager launches a headless OpenCode backend until interrupted.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"bob/container_manager"
)

// invocation is what the command line asks for.
type invocation struct {
	options containermanager.Options
	// config is the configuration file's absolute path.
	config string
	// shared attaches to (or starts) the user's one shared bob instead of starting a private one.
	shared bool
	// broker serves the shared bob; consumers start it with this flag.
	broker   bool
	linger   time.Duration
	consumer string
}

func parseArgs(args []string) (invocation, error) {
	var call invocation
	flags := flag.NewFlagSet("container_manager", flag.ContinueOnError)
	config := flags.String("config", "bob.toml", "Configuration file")
	flags.BoolVar(&call.shared, "shared", false,
		"Use the bob this user already runs, starting it if none does; it stops after the last consumer")
	flags.DurationVar(&call.linger, "linger", defaultLinger,
		"With --shared, how long bob outlives its last consumer")
	flags.StringVar(&call.consumer, "consumer", "cli", "With --shared, a name for this consumer in the broker log")
	flags.BoolVar(&call.broker, "broker", false, "Serve the shared bob (started by --shared consumers)")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: container_manager [--config PATH] [--shared [--linger DURATION] [--consumer NAME]]")
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output(), "\nCreate bob.toml from the repository's bob.example.toml and edit its workspace,")
		fmt.Fprintln(flags.Output(), "or select an existing file with --config PATH.")
	}
	if err := flags.Parse(args); err != nil {
		return call, err
	}
	if flags.NArg() != 0 {
		return call, errors.New("use --config to select a configuration file")
	}
	if call.linger < 0 {
		return call, errors.New("--linger must not be negative")
	}
	input, err := loadConfig(*config)
	call.options = input
	if len(args) == 0 && errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(flags.Output(), "No bob.toml found in the current directory.")
		fmt.Fprintln(flags.Output())
		flags.Usage()
		return call, flag.ErrHelp
	}
	if err != nil {
		return call, err
	}
	call.config, err = filepath.Abs(*config)
	return call, err
}

func getOptions(args []string) (containermanager.Options, error) {
	call, err := parseArgs(args)
	return call.options, err
}

func run() (runErr error) {
	call, err := parseArgs(os.Args[1:])
	if err == flag.ErrHelp {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch {
	case call.broker:
		return runBroker(ctx, call.config, call.options, call.linger)
	case call.shared:
		return runShared(ctx, call.config, call.consumer, call.linger, os.Stdout)
	}
	input := call.options
	manager := &containermanager.Manager{}
	defer func() {
		if err := manager.Stop(context.Background()); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("cleanup: %w", err))
		}
	}()
	connection, err := manager.Start(ctx, input)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	// Connection credentials go to the caller, never the model-provider API key.
	if err := json.NewEncoder(os.Stdout).Encode(connection); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
