// nolint
package aws

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"secrets-init/mocks"
	"secrets-init/pkg/secrets"

	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/aws/aws-sdk-go/service/ssm"
)

func TestSecretsProvider_ResolveSecrets(t *testing.T) {
	tests := []struct {
		name                string
		vars                []string
		mockServiceProvider func(*mocks.SecretsManagerAPI, *mocks.SSMAPI) secrets.Provider
		want                []string
		wantErr             bool
	}{
		{
			name: "get single secret from Secrets Manager",
			vars: []string{
				"test-secret=arn:aws:secretsmanager:12345678",
			},
			want: []string{
				"test-secret=test-secret-value",
			},
			mockServiceProvider: func(mockSM *mocks.SecretsManagerAPI, mockSSM *mocks.SSMAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM, ssm: mockSSM}
				secretName := "arn:aws:secretsmanager:12345678"
				secretValue := "test-secret-value"
				valueInput := secretsmanager.GetSecretValueInput{SecretId: &secretName}
				valueOutput := secretsmanager.GetSecretValueOutput{SecretString: &secretValue}
				mockSM.On("GetSecretValue", &valueInput).Return(&valueOutput, nil)
				return &sp
			},
		},
		{
			name: "get 2 secrets from Secrets Manager",
			vars: []string{
				"test-secret-1=arn:aws:secretsmanager:12345678",
				"non-secret=hello",
				"test-secret-2=arn:aws:secretsmanager:87654321",
			},
			want: []string{
				"non-secret=hello",
				"test-secret-1=test-secret-value-1",
				"test-secret-2=test-secret-value-2",
			},
			mockServiceProvider: func(mockSM *mocks.SecretsManagerAPI, mockSSM *mocks.SSMAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM, ssm: mockSSM}
				vars := map[string]string{
					"arn:aws:secretsmanager:12345678": "test-secret-value-1",
					"arn:aws:secretsmanager:87654321": "test-secret-value-2",
				}
				for n, v := range vars {
					name := n
					value := v
					valueInput := secretsmanager.GetSecretValueInput{SecretId: &name}
					valueOutput := secretsmanager.GetSecretValueOutput{SecretString: &value}
					mockSM.On("GetSecretValue", &valueInput).Return(&valueOutput, nil)
				}
				return &sp
			},
		},
		{
			name: "get all secrets from from Secrets Manager json",
			vars: []string{
				"test-secret-1=arn:aws:secretsmanager:12345678-json",
			},
			want: []string{
				"TEST_1=test-secret-value-1",
				"TEST_2=test-secret-value-2",
			},
			mockServiceProvider: func(mockSM *mocks.SecretsManagerAPI, mockSSM *mocks.SSMAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM, ssm: mockSSM}
				vars := map[string]string{
					"arn:aws:secretsmanager:12345678-json": "{\n  \"TEST_1\": \"test-secret-value-1\",\n  \"TEST_2\": \"test-secret-value-2\"\n}",
				}
				for n, v := range vars {
					name := n
					value := v
					valueInput := secretsmanager.GetSecretValueInput{SecretId: &name}
					valueOutput := secretsmanager.GetSecretValueOutput{SecretString: &value}
					mockSM.On("GetSecretValue", &valueInput).Return(&valueOutput, nil)
				}
				return &sp
			},
		},
		{
			name: "no secrets",
			vars: []string{
				"non-secret-1=hello-1",
				"non-secret-2=hello-2",
			},
			want: []string{
				"non-secret-1=hello-1",
				"non-secret-2=hello-2",
			},
			mockServiceProvider: func(mockSM *mocks.SecretsManagerAPI, mockSSM *mocks.SSMAPI) secrets.Provider {
				return &SecretsProvider{sm: mockSM, ssm: mockSSM}
			},
		},
		{
			name: "error getting secret from Secrets Manager",
			vars: []string{
				"test-secret=arn:aws:secretsmanager:12345678",
				"non-secret=hello",
			},
			want: []string{
				"test-secret=arn:aws:secretsmanager:12345678",
				"non-secret=hello",
			},
			wantErr: true,
			mockServiceProvider: func(mockSM *mocks.SecretsManagerAPI, mockSSM *mocks.SSMAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM, ssm: mockSSM}
				secretName := "arn:aws:secretsmanager:12345678"
				valueInput := secretsmanager.GetSecretValueInput{SecretId: &secretName}
				mockSM.On("GetSecretValue", &valueInput).Return(nil, errors.New("test error"))
				return &sp
			},
		},
		{
			name: "get single secret from SSM Parameter",
			vars: []string{
				"test-secret=arn:aws:ssm:us-east-1:12345678:parameter/secrets/test-secret",
			},
			want: []string{
				"test-secret=test-secret-value",
			},
			mockServiceProvider: func(mockSM *mocks.SecretsManagerAPI, mockSSM *mocks.SSMAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM, ssm: mockSSM}
				secretName := "/secrets/test-secret"
				secretValue := "test-secret-value"
				withDecryption := true
				valueInput := ssm.GetParameterInput{Name: &secretName, WithDecryption: &withDecryption}
				valueOutput := ssm.GetParameterOutput{Parameter: &ssm.Parameter{Value: &secretValue}}
				mockSSM.On("GetParameter", &valueInput).Return(&valueOutput, nil)
				return &sp
			},
		},
		{
			name: "error getting secret from SSM Parameter Store",
			vars: []string{
				"test-secret=arn:aws:ssm:us-east-1:12345678:parameter/secrets/test-secret",
				"non-secret=hello",
			},
			want: []string{
				"test-secret=arn:aws:ssm:us-east-1:12345678:parameter/secrets/test-secret",
				"non-secret=hello",
			},
			wantErr: true,
			mockServiceProvider: func(mockSM *mocks.SecretsManagerAPI, mockSSM *mocks.SSMAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM, ssm: mockSSM}
				secretName := "/secrets/test-secret"
				withDecryption := true
				valueInput := ssm.GetParameterInput{Name: &secretName, WithDecryption: &withDecryption}
				mockSSM.On("GetParameter", &valueInput).Return(nil, errors.New("test error"))
				return &sp
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSM := &mocks.SecretsManagerAPI{}
			mockSSM := &mocks.SSMAPI{}
			sp := tt.mockServiceProvider(mockSM, mockSSM)
			got, err := sp.ResolveSecrets(context.TODO(), tt.vars)
			if (err != nil) != tt.wantErr {
				t.Errorf("SecretsProvider.ResolveSecrets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SecretsProvider.ResolveSecrets() = %v, want %v", got, tt.want)
			}
			mockSM.AssertExpectations(t)
			mockSSM.AssertExpectations(t)
		})
	}
}
