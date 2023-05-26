package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"secrets-init/pkg/secrets" //nolint:gci
	"secrets-init/pkg/secrets/aws"
	"secrets-init/pkg/secrets/google"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix" //nolint:gci
)

var (
	// Version contains the current Version.
	Version = "dev"
	// BuildDate contains a string with the build BuildDate.
	BuildDate = "unknown"
	// GitCommit git commit sha
	GitCommit = "dirty"
	// GitBranch git branch
	GitBranch = "dirty"
	// Platform OS/ARCH
	Platform = ""
)

func main() {
	app := &cli.App{
		Before: setLogFormatter,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "log-format, l",
				Usage:   "select logrus formatter ['json', 'text']",
				Value:   "text",
				EnvVars: []string{"LOG_FORMAT"},
			},
			&cli.StringFlag{
				Name:  "provider, p",
				Usage: "supported secrets manager provider ['aws', 'google']",
				Value: "aws",
			},
		},
		Commands: []*cli.Command{
			{
				Name:      "copy",
				Aliases:   []string{"cp"},
				Usage:     "copy itself to a destination folder",
				ArgsUsage: "destination",
				Action:    copyCmd,
			},
		},
		Name:    "secrets-init",
		Usage:   "enrich environment variables with secrets from secret manager",
		Action:  mainCmd,
		Version: Version,
	}
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("version: %s\n", Version)
		fmt.Printf("  build date: %s\n", BuildDate)
		fmt.Printf("  commit: %s\n", GitCommit)
		fmt.Printf("  branch: %s\n", GitBranch)
		fmt.Printf("  platform: %s\n", Platform)
		fmt.Printf("  built with: %s\n", runtime.Version())
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func copyCmd(c *cli.Context) error {
	if c.Args().Len() != 1 {
		return errors.New("must specify copy destination")
	}
	// full path of current executable
	src := os.Args[0]
	// destination path
	dest := filepath.Join(c.Args().First(), filepath.Base(src))
	// copy file with current file mode flags
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return errors.Wrap(err, "failed to stat source file")
	}
	if !sourceFileStat.Mode().IsRegular() {
		return errors.Errorf("%s is not a regular file", src)
	}
	source, err := os.Open(src)
	if err != nil {
		return errors.Wrap(err, "failed to open source file")
	}
	srcInfo, err := source.Stat()
	if err != nil {
		return errors.Wrap(err, "failed to stat source file")
	}
	defer func() { _ = source.Close() }()
	destination, err := os.Create(dest)
	if err != nil {
		return errors.Wrapf(err, "failed to create %s", dest)
	}
	defer func() { _ = destination.Close() }()
	_, err = io.Copy(destination, source)
	if err != nil {
		return errors.Wrap(err, "failed to copy file")
	}
	err = destination.Chmod(srcInfo.Mode())
	if err != nil {
		return errors.Wrap(err, "failed to set file mode")
	}
	return nil
}

func mainCmd(c *cli.Context) error {
	ctx := context.Background()

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
	var childPid int
	childPid, err = run(ctx, provider, c.Args().Slice())
	if err != nil {
		log.WithError(err).Error("failed to run")
		os.Exit(1)
	}

	// Routine to reap zombies (it's the job of init)
	removeZombies(childPid)
	return nil
}

func removeZombies(childPid int) {
	var exitCode int
	for {
		var status syscall.WaitStatus

		// wait for an orphaned zombie process
		pid, err := syscall.Wait4(-1, &status, 0, nil)

		if pid == -1 {
			// if errno == ECHILD then no children remain; exit cleanly
			if errors.Is(err, syscall.ECHILD) {
				break
			}
			log.WithError(err).Error("unexpected wait4 error")
			os.Exit(1)
		} else {
			// check if pid is child, if so save
			// PID is > 0 if a child was reaped, and we immediately check if another one is waiting
			if pid == childPid {
				exitCode = status.ExitStatus()
			}
			continue
		}
	}
	// no more children, exit with the same code as the child process
	os.Exit(exitCode)
}

// run passed command
func run(ctx context.Context, provider secrets.Provider, commandSlice []string) (childPid int, err error) {
	var commandStr string
	var argsSlice []string

	if len(commandSlice) == 0 {
		log.Warn("no command specified")
		return childPid, err
	}

	// split command and arguments
	commandStr = commandSlice[0]
	// if there is args
	if len(commandSlice) > 1 {
		argsSlice = commandSlice[1:]
	}

	// register a channel to receive system signals
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs)

	// define a command and rebind its stdout and stdin
	cmd := exec.Command(commandStr, argsSlice...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// create a dedicated pidgroup used to forward signals to the main process and its children
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// set environment variables
	if provider != nil {
		cmd.Env, err = provider.ResolveSecrets(ctx, os.Environ())
		if err != nil {
			log.WithError(err).Error("failed to resolve secrets")
		}
	} else {
		log.Warn("no secrets provider available; using environment without resolving secrets")
		cmd.Env = os.Environ()
	}

	// start the specified command
	log.WithFields(log.Fields{
		"command": commandStr,
		"args":    argsSlice,
		"env":     cmd.Env,
	}).Debug("starting command")
	err = cmd.Start()
	if err != nil {
		return childPid, errors.Wrap(err, "failed to start command")
	}
	childPid = cmd.Process.Pid

	// Goroutine for signals forwarding
	go func() {
		for sig := range sigs {
			// ignore:
			// - SIGCHLD signals, since these are only useful for secrets-init
			// - SIGURG signals, since they are used internally by the secrets-init
			//   go runtime (see https://github.com/golang/go/issues/37942) and are of
			//   no interest to the child process
			if sig != syscall.SIGCHLD && sig != syscall.SIGURG {
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

	return childPid, nil
}

func setLogFormatter(c *cli.Context) error {
	if c.String("log-format") == "json" {
		log.SetFormatter(&log.JSONFormatter{})
	} else if c.String("log-format") == "text" {
		log.SetFormatter(&log.TextFormatter{})
	}
	return nil
}
