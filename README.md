# Screwdriver AWS Consumer Service

> Consumer-Service Package for Screwdriver AWS Integration

This is the package for Screwdriver continuous delivery solution that receives and processes build messages.

## Usage

```bash
go install github.com/screwdriver-cd/aws-consumer-service@latest
```

## Executors

### [aws-consumer-service/executor/serverless](github.com/screwdriver-cd/aws-consumer-service/executor/serverless)
This executor is used when annotation in screwdriver.yaml is set to `screwdriver.cd/executor: "sls"`.

### [aws-consumer-service/executor/eks](github.com/screwdriver-cd/aws-consumer-service/executor/eks)
This executor is used when annotation in screwdriver.yaml is set to `screwdriver.cd/executor: "eks"`.