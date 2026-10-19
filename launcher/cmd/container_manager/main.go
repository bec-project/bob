// bob-manager runs the per-user broker that keeps one bob for every GUI a user opens.
package main

import (
	"context"
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
	// broker serves bob itself; `bob-manager` starts it detached when none runs.
	broker bool
	linger time.Duration
}

func parseArgs(args []string) (invocation, error) {
	var call invocation
	flags := flag.NewFlagSet("bob-manager", flag.ContinueOnError)
	config := flags.String("config", "bob.toml", "Configuration file")
	flags.DurationVar(&call.linger, "linger", defaultLinger, "How long bob outlives its last client")
	flags.BoolVar(&call.broker, "broker", false, "Run the broker itself (bob-manager starts it when none runs)")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: bob-manager [--config PATH] [--linger DURATION]")
		fmt.Fprintln(flags.Output(), "\nbob starts from its GUI. bob-manager makes sure this user's bob broker runs, starting it")
		fmt.Fprintln(flags.Output(), "with this configuration when none does, and prints the broker's socket; GUIs and other")
		fmt.Fprintln(flags.Output(), "clients connect there for bob's connection. bob starts with the first client and stops")
		fmt.Fprintln(flags.Output(), "the linger time after the last one leaves.")
		fmt.Fprintln(flags.Output())
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output(), "\nCreate bob.toml from the repository's deploy/bob.example.toml and edit its workspace,")
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

func run() error {
	call, err := parseArgs(os.Args[1:])
	if err == flag.ErrHelp {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if call.broker {
		return runBroker(ctx, call.config, call.options, call.linger)
	}
	return runLocate(ctx, call.config, call.linger, os.Stdout)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
