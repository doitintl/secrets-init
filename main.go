package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"secrets-init/pkg/secrets"
	"secrets-init/pkg/secrets/aws"
	"secrets-init/pkg/secrets/google"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix"
)

var (
	// Version contains the current version.
	version = "dev"
	// BuildDate contains a string with the build date.
	date = "unknown"
	// commit
	commit = "dirty"
	// buildBy
	builtBy = "local"
)

func main() {
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "provider, p",
				Usage: "supported secrets manager provider ['aws', 'google']",
				Value: "aws",
			},
		},
		Name:    "secrets-init",
		Usage:   "enrich environment variables with secrets from secret manager",
		Action:  mainCmd,
		Version: version,
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("secrets-init %s\n", version)
		fmt.Printf("  build date: %s\n", date)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built by: %s\n", builtBy)
		fmt.Printf("  built with: %s\n", runtime.Version())
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func mainCmd(c *cli.Context) error {
	// Routine to reap zombies (it's the job of init)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go removeZombies(ctx, &wg)

	// get provider
	var provider secrets.Provider
	var err error
	if c.String("provider") == "aws" {
		provider, err = aws.NewAwsSecretsProvider()
	} else if c.String("provider") == "google" {
		provider, err = google.NewGoogleSecretsProvider(ctx)
	}
	if err != nil {
		log.WithField("provider", c.String("provider")).WithError(err).Error("failed to initialize secrets provider")
	}
	// Launch main command
	var mainRC int
	err = run(ctx, provider, c.Args().Slice())
	if err != nil {
		log.WithError(err).Error("failed to run")
		mainRC = 1
	}

	// Wait removeZombies goroutine
	cleanQuit(cancel, &wg, mainRC)
	return nil
}

func removeZombies(ctx context.Context, wg *sync.WaitGroup) {
	for {
		var status syscall.WaitStatus

		// wait for an orphaned zombie process
		pid, _ := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)

		if pid <= 0 {
			// PID is 0 or -1 if no child waiting, so we wait for 1 second for next check
			time.Sleep(1 * time.Second)
		} else {
			// PID is > 0 if a child was reaped and we immediately check if another one is waiting
			continue
		}

		// non-blocking test if context is done
		select {
		case <-ctx.Done():
			// context is done, so we stop goroutine
			wg.Done()
			return
		default:
		}
	}
}

// run passed command
func run(ctx context.Context, provider secrets.Provider, commandSlice []string) error {
	var commandStr string
	var argsSlice []string

	if len(commandSlice) == 0 {
		log.Warn("no command specified")
		return nil
	}

	// split command and arguments
	commandStr = commandSlice[0]
	// if there is args
	if len(commandSlice) > 1 {
		argsSlice = commandSlice[1:]
	}

	// register a channel to receive system signals
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs)
	defer signal.Reset()

	// define a command and rebind its stdout and stdin
	cmd := exec.Command(commandStr, argsSlice...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// create a dedicated pidgroup used to forward signals to the main process and its children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// set environment variables
	var err error
	cmd.Env, err = provider.ResolveSecrets(ctx, os.Environ())
	if err != nil {
		log.WithError(err).Error("failed to resolve secrets")
	}

	// start the specified command
	log.WithFields(log.Fields{
		"command": commandStr,
		"args":    argsSlice,
		"env":     cmd.Env,
	}).Debug("starting command")
	err = cmd.Start()
	if err != nil {
		log.WithError(err).Error("failed to start command")
		return err
	}

	// Goroutine for signals forwarding
	go func() {
		for sig := range sigs {
			// ignore SIGCHLD signals since these are only useful for secrets-init
			if sig != syscall.SIGCHLD {
				// forward signal to the main process and its children
				e := syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
				if e != nil {
					log.WithFields(log.Fields{
						"pid":    cmd.Process.Pid,
						"path":   cmd.Path,
						"args":   cmd.Args,
						"signal": unix.SignalName(sig.(syscall.Signal)),
					}).WithError(e).Error("failed to send system signal to the process")
				}
			}
		}
	}()

	// wait for the command to exit
	err = cmd.Wait()
	if err != nil {
		log.WithError(err).Error("failed to wait for command to complete")
		return err
	}

	return nil
}

func cleanQuit(cancel context.CancelFunc, wg *sync.WaitGroup, code int) {
	// signal zombie goroutine to stop and wait for it to release waitgroup
	cancel()
	wg.Wait()

	os.Exit(code)
}
