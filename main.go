package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/ssm"
)

var (
	versionString = "0.1.4"
)

func main() {
	var version bool

	flag.BoolVar(&version, "version", false, "display version")
	flag.Parse()

	if version {
		fmt.Println(versionString)
		os.Exit(0)
	}

	if len(flag.Args()) == 0 {
		log.Fatal("[secrets-init] no command defined, exiting")
	}

	// Routine to reap zombies (it's the job of init)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go removeZombies(ctx, &wg)

	// Launch main command
	var mainRC int
	err := run(flag.Args())
	if err != nil {
		log.Fatalf("[secrets-init] command failed: %s\n", err)
		mainRC = 1
	}

	// Wait removeZombies goroutine
	cleanQuit(cancel, &wg, mainRC)
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

// resolve secrets against AWS Secret Manager and AWS SSM Parameter Store
// replace all ENV variables values prefixed with 'aws:aws:secretsmanager' and 'arn:aws:ssm:REGION:ACCOUNT:parameter'
// by corresponding secrets from AWS Secret Manager and AWS Parameter Store
func resolveSecrets() []string {
	var envs []string
	var s *session.Session
	var sm *secretsmanager.SecretsManager
	var ssmsvc *ssm.SSM

	for _, env := range os.Environ() {
		kv := strings.Split(env, "=")
		key, value := kv[0], kv[1]
		if strings.HasPrefix(value, "arn:aws:secretsmanager") {
			// create AWS API session, if needed
			if s == nil {
				s = session.Must(session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable}))
			}
			// create AWS secret manager, if needed
			if sm == nil {
				sm = secretsmanager.New(s)
			}
			// get secret value
			secret, err := sm.GetSecretValue(&secretsmanager.GetSecretValueInput{SecretId: &value})
			if err == nil {
				env = key + "=" + *secret.SecretString
			}
		} else if strings.HasPrefix(value, "arn:aws:ssm") && strings.Contains(value, ":parameter/") {
			tokens := strings.Split(value, ":")
			// valid parameter ARN arn:aws:ssm:REGION:ACCOUNT:parameter/PATH
			if len(tokens) == 6 {
				// get SSM parameter name (path)
				paramName := strings.TrimPrefix(tokens[5], "parameter")
				// create AWS API session, if needed
				if s == nil {
					s = session.Must(session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable}))
				}
				// create SSM service, if needed
				if ssmsvc == nil {
					ssmsvc = ssm.New(s)
				}
				withDecryption := true
				param, err := ssmsvc.GetParameter(&ssm.GetParameterInput{
					Name:           &paramName,
					WithDecryption: &withDecryption,
				})
				if err == nil {
					env = key + "=" + *param.Parameter.Value
				}
			}
		}
		envs = append(envs, env)
	}

	return envs
}

// run passed command
func run(commandSlice []string) error {
	var commandStr string
	var argsSlice []string

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
	cmd.Env = resolveSecrets()

	// Goroutine for signals forwarding
	go func() {
		for sig := range sigs {
			// ignore SIGCHLD signals since these are only usefull for secrets-init
			if sig != syscall.SIGCHLD {
				// forward signal to the main process and its children
				syscall.Kill(-cmd.Process.Pid, sig.(syscall.Signal))
			}
		}
	}()

	// start the specified command
	err := cmd.Start()
	if err != nil {
		return err
	}

	// wait for the command to exit
	err = cmd.Wait()
	if err != nil {
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
