package google

import (
	"context"

	"github.com/googleapis/gax-go/v2"
	secretspb "google.golang.org/genproto/googleapis/cloud/secrets/v1beta1"
)

type GoogleSecretsManagerAPI interface {
	AccessSecretVersion(ctx context.Context, req *secretspb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretspb.AccessSecretVersionResponse, error)
}
