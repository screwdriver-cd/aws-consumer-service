module github.com/screwdriver-cd/aws-consumer-service

go 1.15

require (
	github.com/aws/aws-lambda-go v1.26.0
	github.com/aws/aws-sdk-go v1.40.13
	github.com/google/uuid v1.3.0 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.0
	github.com/stretchr/testify v1.6.1
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/api v0.19.0
	k8s.io/apimachinery v0.19.0
	k8s.io/client-go v0.19.0
	sigs.k8s.io/aws-iam-authenticator v0.5.3
)
