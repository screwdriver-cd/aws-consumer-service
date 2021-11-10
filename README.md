# Screwdriver AWS Consumer Service
[![Build Status][build-image]][build-url]
[![Open Issues][issues-image]][issues-url]
[![Latest Release][version-image]][version-url]
[![Go Report Card][goreport-image]][goreport-url]
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


[version-image]: https://img.shields.io/github/tag/screwdriver-cd/aws-consumer-service.svg
[version-url]: https://github.com/screwdriver-cd/aws-consumer-service/releases
[issues-image]: https://img.shields.io/github/issues/screwdriver-cd/screwdriver.svg
[issues-url]: https://github.com/screwdriver-cd/screwdriver/issues
[build-image]: https://cd.screwdriver.cd/pipelines/7970/badge
[build-url]: https://cd.screwdriver.cd/pipelines/7970/events
[goreport-image]: https://goreportcard.com/badge/github.com/Screwdriver-cd/aws-consumer-service
[goreport-url]: https://goreportcard.com/report/github.com/Screwdriver-cd/aws-consumer-service
