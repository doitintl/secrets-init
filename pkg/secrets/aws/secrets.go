package aws

import (
	"context"
	"encoding/json"
	"strings"

	"secrets-init/pkg/secrets" //nolint:gci

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/pkg/errors" //nolint:gci
)

const (
	paramNameTokens            = 6
	paramNameTokensWithVersion = 7
	maxSpitSize                = 2
)

// SecretsProvider AWS secrets provider
type SecretsProvider struct {
	session *session.Session
	sm      secretsmanageriface.SecretsManagerAPI
	ssm     ssmiface.SSMAPI
}

// NewAwsSecretsProvider init AWS Secrets Provider
func NewAwsSecretsProvider() (secrets.Provider, error) {
	var err error
	sp := SecretsProvider{}
	// create AWS session
	sp.session, err = session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create AWS session")
	}
	// init AWS Secrets Manager client
	sp.sm = secretsmanager.New(sp.session)
	// init AWS SSM client
	sp.ssm = ssm.New(sp.session)
	return &sp, nil
}

// ResolveSecrets replaces all passed variables values prefixed with 'aws:aws:secretsmanager' and 'arn:aws:ssm:REGION:ACCOUNT:parameter'
// by corresponding secrets from AWS Secret Manager and AWS Parameter Store
func (sp *SecretsProvider) ResolveSecrets(_ context.Context, vars []string) ([]string, error) { //nolint:gocognit
	envs := make([]string, 0, len(vars))

	for _, env := range vars {
		kv := strings.SplitN(env, "=", maxSpitSize)
		key, value := kv[0], kv[1]
		if strings.HasPrefix(value, "arn:aws:secretsmanager") {
			// get secret value
			secret, err := sp.sm.GetSecretValue(&secretsmanager.GetSecretValueInput{SecretId: &value})
			if err != nil {
				return vars, errors.Wrap(err, "failed to get secret from AWS Secrets Manager")
			}
			if IsJSON(secret.SecretString) {
				var keyValueSecret map[string]string
				err = json.Unmarshal([]byte(*secret.SecretString), &keyValueSecret)
				if err != nil {
					return vars, errors.Wrap(err, "failed to decode key/value secret")
				}
				for key, value := range keyValueSecret {
					e := key + "=" + value
					envs = append(envs, e)
				}
				continue // We continue to not add this ENV variable but only the environment variables that exists in the JSON
			} else {
				env = key + "=" + *secret.SecretString
			}
		} else if strings.HasPrefix(value, "arn:aws:ssm") && strings.Contains(value, ":parameter/") {
			tokens := strings.Split(value, ":")
			// valid parameter ARN arn:aws:ssm:REGION:ACCOUNT:parameter/PATH
			// or arn:aws:ssm:REGION:ACCOUNT:parameter/PATH:VERSION
			if len(tokens) == paramNameTokens || len(tokens) == paramNameTokensWithVersion {
				// get SSM parameter name (path)
				paramName := strings.TrimPrefix(tokens[5], "parameter")

				if len(tokens) == paramNameTokensWithVersion {
					paramName = paramName + ":" + tokens[6]
				}

				// get AWS SSM API
				withDecryption := true
				param, err := sp.ssm.GetParameter(&ssm.GetParameterInput{
					Name:           &paramName,
					WithDecryption: &withDecryption,
				})
				if err != nil {
					return vars, errors.Wrap(err, "failed to get secret from AWS Parameters Store")
				}
				env = key + "=" + *param.Parameter.Value
			}
		}
		envs = append(envs, env)
	}

	return envs, nil
}

func IsJSON(str *string) bool {
	if str == nil {
		return false
	}
	var js json.RawMessage
	return json.Unmarshal([]byte(*str), &js) == nil
}
