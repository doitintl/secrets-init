package aws

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/ssm"

	log "github.com/sirupsen/logrus"
)

// SecretsProvider AWS secrets provider
type SecretsProvider struct{}

// ResolveSecrets replaces all passed variables values prefixed with 'aws:aws:secretsmanager' and 'arn:aws:ssm:REGION:ACCOUNT:parameter'
// by corresponding secrets from AWS Secret Manager and AWS Parameter Store
func (sp *SecretsProvider) ResolveSecrets(ctx context.Context, vars []string) []string {
	var envs []string
	var s *session.Session
	var sm *secretsmanager.SecretsManager
	var ssmsvc *ssm.SSM

	for _, env := range vars {
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
			if err != nil {
				log.WithError(err).Error("failed to get secret from AWS Secrets Manager")
				continue
			}
			env = key + "=" + *secret.SecretString
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
				if err != nil {
					log.WithError(err).Error("failed to get secret from AWS Parameters Store")
					continue
				}
				env = key + "=" + *param.Parameter.Value
			}
		}
		envs = append(envs, env)
	}

	return envs
}
