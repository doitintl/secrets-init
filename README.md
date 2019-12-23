[![Build Status](https://github.com/doitintl/secrets-init/workflows/goreleaser/badge.svg)](https://github.com/doitintl/secrets-init/actions?query=workflow%3Agoreleaser) [![Go Report Card](https://goreportcard.com/badge/github.com/doitintl/secrets-init)](https://goreportcard.com/report/github.com/doitintl/secrets-init)

[![](https://images.microbadger.com/badges/image/doitintl/secrets-init.svg)](http://microbadger.com/images/doitintl/secrets-init) [![](https://images.microbadger.com/badges/version/doitintl/secrets-init.svg)](http://microbadger.com/images/doitintl/secrets-init) [![](https://images.microbadger.com/badges/commit/doitintl/secrets-init.svg)](http://microbadger.com/images/doitintl/secrets-init)

# secrets-init

`secrets-init` is a minimalistic init system designed to run as PID 1 inside container environments, similar to [dumb-init](https://github.com/Yelp/dumb-init), integrated with multiple secrets manager services:

- [AWS Secrets Manager](https://aws.amazon.com/secrets-manager/)
- [AWS Systems Manager Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html)
- [Google Secret Manager](https://cloud.google.com/secret-manager/docs/)

## Why you need an init system

Please [read Yelp *dumb-init* repo explanation](https://github.com/Yelp/dumb-init/blob/v1.2.0/README.md#why-you-need-an-init-system)

Summary:

- Proper signal forwarding
- Orphaned zombies reaping

## What `secrets-init` does

`secrets-init` runs as `PID 1`, acting like a simple init system. It launches a single process and then proxies all received signals to a session rooted at that child process.

`secrets-init` also passes almost all environment variables without modification, replacing _secret variables_ with values from secret management services.

### Integration with AWS Secrets Manager

User can put AWS secret ARN as environment variable value. The `secrets-init` will resolve any environment value, using specified ARN, to referenced secret value.

```sh
# environment variable passed to `secrets-init`
MY_DB_PASSWORD=arn:aws:secretsmanager:$AWS_REGION:$AWS_ACCOUNT_ID:secret:mydbpassword-cdma3

# environment variable passed to child process, resolved by `secrets-init`
MY_DB_PASSWORD=very-secret-password
```

### Integration with AWS Systems Manager Parameter Store

It is possible to use AWS Systems Manager Parameter Store to store application parameters and secrets.

User can put AWS Parameter Store ARN as environment variable value. The `secrets-init` will resolve any environment value, using specified ARN, to referenced parameter value.

```sh
# environment variable passed to `secrets-init`
MY_API_KEY=arn:aws:ssm:$AWS_REGION:$AWS_ACCOUNT_ID:parameter/api/key

# environment variable passed to child process, resolved by `secrets-init`
MY_API_KEY=key-123456789
```

### Integration with Google Secret Manager

User can put Google secret name (prefixed with `gcp:secretmanager:`) as environment variable value. The `secrets-init` will resolve any environment value, using specified name, to referenced secret value.

```sh
# environment variable passed to `secrets-init`
MY_DB_PASSWORD=gcp:secretmanager:projects/$PROJECT_ID/secrets/mydbpassword
# OR versioned secret (with version or 'latest')
MY_DB_PASSWORD=gcp:secretmanager:projects/$PROJECT_ID/secrets/mydbpassword/versions/2

# environment variable passed to child process, resolved by `secrets-init`
MY_DB_PASSWORD=very-secret-password
```

### Requirement

In order to resolve AWS secrets from AWS Secret Manager and Parameter Store, `secrets-init` should run under IAM role that has permission to access desired secrets.

This can be achieved by assigning IAM Role to Kubernetes Pod or ECS Task. It's possible to assign IAM Role to EC2 instance, where container is running, but this option is less secure.

## Code Reference

Initial init system code was copied from [go-init](https://github.com/pablo-ruth/go-init) project.
