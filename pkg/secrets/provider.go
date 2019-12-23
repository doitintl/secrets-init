package secrets

import "context"

// Provider secrets provider interface
type Provider interface {
	ResolveSecrets(ctx context.Context, envs []string) []string
}
