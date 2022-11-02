// nolint
package google

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"secrets-init/mocks"
	"secrets-init/pkg/secrets"

	secretspb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

func TestSecretsProvider_ResolveSecrets(t *testing.T) {
	type fields struct {
		sm SecretsManagerAPI
	}
	type args struct {
		ctx  context.Context
		vars []string
	}
	tests := []struct {
		name                string
		fields              fields
		args                args
		mockServiceProvider func(context.Context, *mocks.GoogleSecretsManagerAPI) secrets.Provider
		want                []string
		wantErr             bool
	}{
		{
			name: "get implicit (latest) version single secret from Secrets Manager",
			args: args{
				ctx: context.TODO(),
				vars: []string{
					"test-secret=gcp:secretmanager:projects/test-project-id/secrets/test-secret",
				},
			},
			want: []string{
				"test-secret=test-secret-value",
			},
			mockServiceProvider: func(ctx context.Context, mockSM *mocks.GoogleSecretsManagerAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM}
				req := secretspb.AccessSecretVersionRequest{
					Name: "projects/test-project-id/secrets/test-secret/versions/latest",
				}
				res := secretspb.AccessSecretVersionResponse{Payload: &secretspb.SecretPayload{
					Data: []byte("test-secret-value"),
				}}
				mockSM.On("AccessSecretVersion", ctx, &req).Return(&res, nil)
				return &sp
			},
		},
		{
			name: "get explicit single secret version from Secrets Manager",
			args: args{
				ctx: context.TODO(),
				vars: []string{
					"test-secret=gcp:secretmanager:projects/test-project-id/secrets/test-secret/versions/5",
				},
			},
			want: []string{
				"test-secret=test-secret-value",
			},
			mockServiceProvider: func(ctx context.Context, mockSM *mocks.GoogleSecretsManagerAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM}
				req := secretspb.AccessSecretVersionRequest{
					Name: "projects/test-project-id/secrets/test-secret/versions/5",
				}
				res := secretspb.AccessSecretVersionResponse{Payload: &secretspb.SecretPayload{
					Data: []byte("test-secret-value"),
				}}
				mockSM.On("AccessSecretVersion", ctx, &req).Return(&res, nil)
				return &sp
			},
		},
		{
			name: "get 2 secrets from Secrets Manager",
			args: args{
				ctx: context.TODO(),
				vars: []string{
					"test-secret-1=gcp:secretmanager:projects/test-project-id/secrets/test-secret/versions/5",
					"non-secret=hello",
					"test-secret-2=gcp:secretmanager:projects/test-project-id/secrets/test-secret",
				},
			},
			want: []string{
				"test-secret-1=test-secret-value-1",
				"non-secret=hello",
				"test-secret-2=test-secret-value-2",
			},
			mockServiceProvider: func(ctx context.Context, mockSM *mocks.GoogleSecretsManagerAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM}
				vars := map[string]string{
					"projects/test-project-id/secrets/test-secret/versions/5":      "test-secret-value-1",
					"projects/test-project-id/secrets/test-secret/versions/latest": "test-secret-value-2",
				}
				for n, v := range vars {
					name := n
					value := v
					req := secretspb.AccessSecretVersionRequest{
						Name: name,
					}
					res := secretspb.AccessSecretVersionResponse{Payload: &secretspb.SecretPayload{
						Data: []byte(value),
					}}
					mockSM.On("AccessSecretVersion", ctx, &req).Return(&res, nil)
				}
				return &sp
			},
		},
		{
			name: "no secrets",
			args: args{
				ctx: context.TODO(),
				vars: []string{
					"non-secret-1=hello-1",
					"non-secret-2=hello-2",
				},
			},
			want: []string{
				"non-secret-1=hello-1",
				"non-secret-2=hello-2",
			},
			mockServiceProvider: func(ctx context.Context, mockSM *mocks.GoogleSecretsManagerAPI) secrets.Provider {
				return &SecretsProvider{sm: mockSM}
			},
		},
		{
			name: "error getting secret from Secrets Manager",
			args: args{
				ctx: context.TODO(), vars: []string{
					"test-secret=gcp:secretmanager:projects/test-project-id/secrets/test-secret",
					"non-secret=hello",
				},
			},
			want: []string{
				"test-secret=gcp:secretmanager:projects/test-project-id/secrets/test-secret",
				"non-secret=hello",
			},
			wantErr: true,
			mockServiceProvider: func(ctx context.Context, mockSM *mocks.GoogleSecretsManagerAPI) secrets.Provider {
				sp := SecretsProvider{sm: mockSM}
				req := secretspb.AccessSecretVersionRequest{
					Name: "projects/test-project-id/secrets/test-secret/versions/latest",
				}
				mockSM.On("AccessSecretVersion", ctx, &req).Return(nil, errors.New("test error"))
				return &sp
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSM := &mocks.GoogleSecretsManagerAPI{}
			sp := tt.mockServiceProvider(tt.args.ctx, mockSM)
			got, err := sp.ResolveSecrets(tt.args.ctx, tt.args.vars)
			if (err != nil) != tt.wantErr {
				t.Errorf("SecretsProvider.ResolveSecrets() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SecretsProvider.ResolveSecrets() = %v, want %v", got, tt.want)
			}
			mockSM.AssertExpectations(t)
		})
	}
}
