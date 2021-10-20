/*
Copyright 2016 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Note: the example only works with the code within the same release/branch.
package kubernetes

import (
	"context"
	"secrets-init/pkg/secrets"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/pkg/errors"
)

// Secrets Provider Kubernetes secrets provider
type SecretsProvider struct {
	kubernetes.Clientset
}

// NewKubernetesSecretsProvider init Kubernetes Secrets Provider
func NewKubernetesSecretsProvider(ctx context.Context) (secrets.Provider, error) {
	var err error

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to acquire in cluster config")
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to establish clientset")
	}

	return &SecretsProvider{
		Clientset: *clientset,
	}, nil
}

// ResolveSecrets replaces all passed in variables values prefixed with 'k8s:secret:'
// by corresponding secrets from the Kubernetes cluster the workload runs.
// Format: `k8s:secret:{NAMESPACE}:{SECRET_NAME}:{SECRET_KEY}`
func (sp *SecretsProvider) ResolveSecrets(ctx context.Context, vars []string) ([]string, error) {
	var envs []string

	for _, env := range vars {
		kv := strings.Split(env, "=")
		key, value := kv[0], kv[1]
		if strings.HasPrefix(value, "k8s:secret:") {
			metadata := strings.Split(value, ":")
			namespace, name, secretkey := metadata[2], metadata[3], metadata[4]
			secret, err := sp.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return vars, errors.Wrap(err, "failed to get secrets from Kubernetes")
			}
			env = key + "=" + string(secret.Data[secretkey])
		}
		envs = append(envs, env)
	}

	return envs, nil
}
