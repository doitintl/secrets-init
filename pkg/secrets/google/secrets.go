package google

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"secrets-init/pkg/secrets" //nolint:gci

	"cloud.google.com/go/compute/metadata"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	secretspb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1" //nolint:gci
)

var fullSecretRe = regexp.MustCompile(`projects/[^/]+/secrets/[^/+](/version/[^/+])?`)

type result struct {
	Env string
	Err error
}

// SecretsProvider Google Cloud secrets provider
type SecretsProvider struct {
	sm        SecretsManagerAPI
	projectID string
}

// NewGoogleSecretsProvider init Google Secrets Provider
func NewGoogleSecretsProvider(ctx context.Context, projectID string) (secrets.Provider, error) {
	sp := SecretsProvider{}
	var err error

	if projectID != "" {
		sp.projectID = projectID
	} else {
		sp.projectID, err = metadata.ProjectID()
		if err != nil {
			log.WithError(err).Infoln("The Google project cannot be detected, you won't be able to use the short secret version")
		}
	}

	sp.sm, err = secretmanager.NewClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize Google Cloud SDK")
	}
	return &sp, nil
}

// ResolveSecrets replaces all passed variables values prefixed with 'gcp:secretmanager'
// by corresponding secrets from Google Secret Manager
// The secret name should be in the format (optionally with version)
//
//	`gcp:secretmanager:projects/{PROJECT_ID}/secrets/{SECRET_NAME}`
//	`gcp:secretmanager:projects/{PROJECT_ID}/secrets/{SECRET_NAME}/versions/{VERSION|latest}`
//	`gcp:secretmanager:{SECRET_NAME}
//	`gcp:secretmanager:{SECRET_NAME}/versions/{VERSION|latest}`
func (sp SecretsProvider) ResolveSecrets(ctx context.Context, vars []string) ([]string, error) {
	envs := make([]string, 0, len(vars))

	// Create a channel to collect the results
	results := make(chan result, len(vars))

	// Start a goroutine for each secret
	var wg sync.WaitGroup
	for _, env := range vars {
		wg.Add(1)
		go func(env string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				results <- result{Err: ctx.Err()}
				return
			default:
				val, err := sp.processEnvironmentVariable(ctx, env)
				if err != nil {
					results <- result{Err: err}
					return
				}
				results <- result{Env: val}
			}
		}(env)
	}

	// Start another goroutine to close the results channel when all fetch goroutines are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect the results
	for res := range results {
		if res.Err != nil {
			return vars, res.Err
		}
		envs = append(envs, res.Env)
	}

	return envs, nil
}

// processEnvironmentVariable processes the environment variable and replaces the value with the secret value
func (sp SecretsProvider) processEnvironmentVariable(ctx context.Context, env string) (string, error) {
	kv := strings.Split(env, "=")
	key, value := kv[0], kv[1]
	if !strings.HasPrefix(value, "gcp:secretmanager:") {
		return env, nil
	}

	// construct valid secret name
	name := strings.TrimPrefix(value, "gcp:secretmanager:")

	isLong := fullSecretRe.MatchString(name)

	if !isLong {
		if sp.projectID == "" {
			return "", errors.Errorf("failed to get secret \"%s\" from Google Secret Manager (unknown project)", name)
		}
		name = fmt.Sprintf("projects/%s/secrets/%s", sp.projectID, name)
	}

	// if no version specified add latest
	if !strings.Contains(name, "/versions/") {
		name += "/versions/latest"
	}

	// get secret value
	req := &secretspb.AccessSecretVersionRequest{
		Name: name,
	}
	secret, err := sp.sm.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to get secret from Google Secret Manager: %w", err)
	}
	return key + "=" + string(secret.Payload.GetData()), nil
}
