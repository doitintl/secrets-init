package aws

import (
	"context"
	"secrets-init/pkg/secrets"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/secretsmanager/secretsmanageriface"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
	"github.com/pkg/errors"
	"encoding/json"
	"github.com/tidwall/gjson"
)

// SecretsProvider AWS secrets provider
type SecretsProvider struct {
	session *session.Session
	sm      secretsmanageriface.SecretsManagerAPI
	ssm     ssmiface.SSMAPI
}

func isJSON(s string) bool {
	var js map[string]interface{}
	return json.Unmarshal([]byte(s), &js) == nil
}

// NewAwsSecretsProvider init AWS Secrets Provider
func NewAwsSecretsProvider() (secrets.Provider, error) {
	var err error
	sp := SecretsProvider{}
	// create AWS session
	sp.session, err = session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable})
	if err != nil {
		return nil, err
	}
	// init AWS Secrets Manager client
	sp.sm = secretsmanager.New(sp.session)
	// init AWS SSM client
	sp.ssm = ssm.New(sp.session)
	return &sp, nil
}

// ResolveSecrets replaces all passed variables values prefixed with 'aws:aws:secretsmanager' and 'arn:aws:ssm:REGION:ACCOUNT:parameter'
// by corresponding secrets from AWS Secret Manager and AWS Parameter Store
func (sp *SecretsProvider) ResolveSecrets(ctx context.Context, vars []string) ([]string, error) {
	var envs []string

	for _, env := range vars {
		kv := strings.Split(env, "=")
		key, value := kv[0], kv[1]
		if strings.HasPrefix(value, "arn:aws:secretsmanager") {
			vals := strings.Split(value, "#")
			value = vals[0] // secret ARN
			k := vals[len(vals)-1] // key to extract from SecretString
			// get secret value
			secret, err := sp.sm.GetSecretValue(&secretsmanager.GetSecretValueInput{SecretId: &value})
			if err != nil {
				return vars, errors.Wrap(err, "failed to get secret from AWS Secrets Manager")
			}
			if isJSON(*secret.SecretString) {
				v := gjson.Get(*secret.SecretString, k)
				env = key + "=" + v.String()
			} else {
				env = key + "=" + *secret.SecretString
			}
		} else if strings.HasPrefix(value, "arn:aws:ssm") && strings.Contains(value, ":parameter/") {
			tokens := strings.Split(value, ":")
			// valid parameter ARN arn:aws:ssm:REGION:ACCOUNT:parameter/PATH
			// or arn:aws:ssm:REGION:ACCOUNT:parameter/PATH:VERSION
			if len(tokens) == 6 || len(tokens) == 7 {
				// get SSM parameter name (path)
				paramName := strings.TrimPrefix(tokens[5], "parameter")

				if len(tokens) == 7 {
					paramName = strings.Join([]string{paramName, tokens[6]}, ":")
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
