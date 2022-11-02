package google

import (
	"context"

	"github.com/googleapis/gax-go/v2"
	secretspb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

// SecretsManagerAPI is the interface for the Google Secrets Manager API.
type SecretsManagerAPI interface {
	AccessSecretVersion(ctx context.Context, req *secretspb.AccessSecretVersionRequest, opts ...gax.CallOption) (*secretspb.AccessSecretVersionResponse, error) //nolint:lll
}
