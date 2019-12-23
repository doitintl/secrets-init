package google

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"

	secretmanager "cloud.google.com/go/secretmanager/apiv1beta1"
	secretspb "google.golang.org/genproto/googleapis/cloud/secrets/v1beta1"
)

// SecretsProvider Google Cloud secrets provider
type SecretsProvider struct{}

// ResolveSecrets replaces all passed variables values prefixed with 'gsp:secretsmanager'
// by corresponding secrets from Google Secret Manager
// The secret name should be in the format (optionally with version)
//    `gcp:secretmanager:projects/{PROJECT_ID}/secrets/{SECRET_NAME}`
//    `gcp:secretmanager:projects/{PROJECT_ID}/secrets/{SECRET_NAME}/versions/{VERSION|latest}`
func (sp SecretsProvider) ResolveSecrets(ctx context.Context, vars []string) []string {
	var envs []string
	var sm *secretmanager.Client
	var err error
	for _, env := range vars {
		kv := strings.Split(env, "=")
		key, value := kv[0], kv[1]
		if strings.HasPrefix(value, "gcp:secretmanager:") {
			if sm == nil {
				sm, err = secretmanager.NewClient(ctx)
				if err != nil {
					log.WithError(err).Error("failed to initialize Google Cloud SDK")
					continue
				}
			}
			// construct valid secret name
			name := strings.TrimPrefix(value, "gcp:secretmanager:")
			// if no version specified add latest
			if !strings.Contains(name, "/versions/") {
				name = name + "/versions/latest"
			}
			// get secret value
			req := &secretspb.AccessSecretVersionRequest{
				Name: name,
			}
			secret, err := sm.AccessSecretVersion(ctx, req)
			if err != nil {
				log.WithError(err).Error("failed to get secret from Google Secret Manager")
				continue
			}
			env = key + "=" + string(secret.Payload.GetData())
		}
		envs = append(envs, env)
	}

	return envs
}
