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
	"syscall"

	"bob/container_manager"
)

func getOptions(args []string) (containermanager.Options, error) {
	flags := flag.NewFlagSet("container_manager", flag.ContinueOnError)
	config := flags.String("config", "bob.toml", "Configuration file")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: container_manager [--config PATH]")
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output(), "\nCreate bob.toml from the repository's bob.example.toml and edit its workspace,")
		fmt.Fprintln(flags.Output(), "or select an existing file with --config PATH.")
	}
	if err := flags.Parse(args); err != nil {
		return containermanager.Options{}, err
	}
	if flags.NArg() != 0 {
		return containermanager.Options{}, errors.New("use --config to select a configuration file")
	}
	input, err := loadConfig(*config)
	if len(args) == 0 && errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(flags.Output(), "No bob.toml found in the current directory.")
		fmt.Fprintln(flags.Output())
		flags.Usage()
		return input, flag.ErrHelp
	}
	return input, err
}

func run() (runErr error) {
	input, err := getOptions(os.Args[1:])
	if err == flag.ErrHelp {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
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
