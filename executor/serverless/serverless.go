package sls

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/aws/aws-sdk-go/service/s3"
)

type ExecutorAwsServerless interface {
	Start(config map[string]interface{}) error
	Stop(config map[string]interface{}) error
}
type awsServerless struct {
	svcCBClient *codebuild.CodeBuild
	svcS3Client *s3.S3
	name        string
}

func (e *awsServerless) checkLauncherUpdate(launcherVersion string) (l bool) {
	listResult, err := e.svcS3Client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(os.Getenv("SD_SLS_BUILD_BUCKET")),
	})
	if err != nil {
		log.Printf("Failed to get launcher version %v", err.Error())
	}

	var launcherUpdate bool = true
	sourceIdentifier := "sdinit-" + launcherVersion
	// check if launcher version already exists
	if len(listResult.Contents) > 0 {
		for _, obj := range listResult.Contents {
			if *obj.Key == sourceIdentifier {
				launcherUpdate = false
			}
		}
	}
	return launcherUpdate
}

// Create a codebuild project and Start a build
func (e *awsServerless) Start(config map[string]interface{}) (string, error) {
	launcherVersion := config["launcherVersion"].(string)
	launcherUpdate := e.checkLauncherUpdate(launcherVersion)
	sourceIdentifier := "sdinit-" + launcherVersion
	project := config["jobName"].(string) + "-" + config["jobId"].(string)
	var securityGroupIds []*string
	for _, sg := range config["securityGroupIds"].([]string) {
		securityGroupIds = append(securityGroupIds, aws.String(sg))
	}
	var subnets []*string
	for _, sn := range config["subnets"].([]string) {
		subnets = append(subnets, aws.String(sn))
	}
	queuedTimeout := config["queuedTimeout"].(int64)
	buildTimeout := config["buildTimeout"].(int64)

	var names []*string
	names = append(names, aws.String(project))
	batchResult, err := e.svcCBClient.BatchGetProjects(&codebuild.BatchGetProjectsInput{
		Names: names,
	})

	mainBuildspec := `version: 0.2\nphases:\n  install:\n    commands:\n      - mkdir /opt/sd && cp -r $CODEBUILD_SRC_DIR_sdinit_sdinit/opt/sd/* /opt/sd/\n  build:\n    commands:\n      - /opt/sd/launcher_entrypoint.sh /opt/sd/run.sh $TOKEN $API $STORE $TIMEOUT $SDBUILDID $UI`
	batchBuildSpec := `version: 0.2
batch:
	fast-fail: false
	build-graph:
	- identifier: init
		env:
		type: LINUX_CONTAINER
		image: ` + config["launcherImage"].(string) + `
		compute-type: ` + config["launcherComputeType"].(string) + `
		privileged-mode: false
		ignore-failure: false
	- identifier: main
		buildspec: ` + `"` + mainBuildspec + `"` + `
		depend-on:
		- init
artifacts:
	base-directory: /opt
	files:  
	- '/opt/**/*'`

	singleBuildSpec := `version: 0.2
phases:
	install:
	commands:
		- mkdir /opt/sd && cp -r $CODEBUILD_SRC_DIR/opt/sd/* /opt/sd/
	build:
	commands:
		- /opt/sd/launcher_entrypoint.sh /opt/sd/run.sh $TOKEN $API $STORE $TIMEOUT $SDBUILDID $UI`

	initArtifact := &codebuild.ProjectArtifacts{
		EncryptionDisabled:   aws.Bool(false),
		Location:             aws.String(os.Getenv("SD_SLS_BUILD_BUCKET")),
		OverrideArtifactName: aws.Bool(false),
		Packaging:            aws.String("ZIP"),
		Type:                 aws.String("S3"),
		Name:                 aws.String(config["launcherVersion"].(string)),
	}

	createRequest := &codebuild.CreateProjectInput{
		Artifacts: &codebuild.ProjectArtifacts{
			Type: aws.String("NO_ARTIFACTS"),
		},
		Name: aws.String(project),
		Source: &codebuild.ProjectSource{
			Buildspec: aws.String(singleBuildSpec),
			Location:  aws.String(os.Getenv("SD_SLS_BUILD_BUCKET") + "/" + sourceIdentifier),
			Type:      aws.String("S3"),
		},
		QueuedTimeoutInMinutes: aws.Int64(queuedTimeout),
		ServiceRole:            aws.String(config["roleArn"].(string)),
		TimeoutInMinutes:       aws.Int64(buildTimeout),
		ConcurrentBuildLimit:   aws.Int64(2),
		Environment: &codebuild.ProjectEnvironment{
			ComputeType:              aws.String(config["computeType"].(string)),
			Image:                    aws.String(config["container"].(string)),
			ImagePullCredentialsType: aws.String(config["imagePullCredentialsType"].(string)),
			PrivilegedMode:           aws.Bool(config["privilegedMode"].(bool)),
			Type:                     aws.String(config["environmentType"].(string)),
		},
		VpcConfig: &codebuild.VpcConfig{
			SecurityGroupIds: securityGroupIds,
			Subnets:          subnets,
			VpcId:            aws.String(config["vpcId"].(string)),
		},
		LogsConfig: &codebuild.LogsConfig{
			CloudWatchLogs: &codebuild.CloudWatchLogsConfig{
				Status: aws.String("DISABLED"),
			},
			S3Logs: &codebuild.S3LogsConfig{
				Status: aws.String("DISABLED"),
			},
		},
		EncryptionKey: aws.String(os.Getenv("SD_SLS_BUILD_ENCRYPTION_KEY")),
	}

	if launcherUpdate {
		createRequest.Source = &codebuild.ProjectSource{
			Buildspec: aws.String(batchBuildSpec),
			Type:      aws.String("NO_SOURCE"),
		}
		createRequest.BuildBatchConfig = &codebuild.ProjectBuildBatchConfig{
			CombineArtifacts: aws.Bool(false),
			Restrictions:     &codebuild.BatchRestrictions{MaximumBuildsAllowed: aws.Int64(2)},
			ServiceRole:      aws.String(config["roleArn"].(string)),
			TimeoutInMins:    aws.Int64(buildTimeout),
		}
	}
	if config["dlc "].(bool) {
		createRequest.Cache = &codebuild.ProjectCache{
			Location: new(string),
			Modes:    []*string{aws.String("LOCAL_DOCKER_LAYER_CACHE")},
			Type:     aws.String("LOCAL"),
		}
		// this need to be set true for dlc
		createRequest.Environment.PrivilegedMode = aws.Bool(true)
	}
	var projectArn string
	if len(batchResult.Projects) == 0 {
		log.Printf("Project does not exist, creating project")
		createResult, err := e.svcCBClient.CreateProject(createRequest)
		if err != nil {
			return "", fmt.Errorf("Got error while creating projects: %v", err)
		}
		log.Printf("Project created: %v", createResult.Project.Arn)
		projectArn = *createResult.Project.Arn
	} else {
		log.Printf("Project already exists, updating project")
		updateRequest := codebuild.UpdateProjectInput(*createRequest)
		updateResult, err := e.svcCBClient.UpdateProject(&updateRequest)
		if err != nil {
			return "", fmt.Errorf("Got error while updating projects: %v", err)
		}
		log.Printf("Project updated: %v", updateResult.Project.Arn)
		projectArn = *updateResult.Project.Arn
	}

	var envVars []*codebuild.EnvironmentVariable = []*codebuild.EnvironmentVariable{
		{Name: aws.String("TOKEN"), Value: aws.String(config["token"].(string))},
		{Name: aws.String("API"), Value: aws.String(config["apiUri"].(string))},
		{Name: aws.String("STORE"), Value: aws.String(config["storeUri"].(string))},
		{Name: aws.String("UI"), Value: aws.String(config["uiUri"].(string))},
		{Name: aws.String("TIMEOUT"), Value: aws.String(config["buildTimeout"].(string))},
		{Name: aws.String("SDBUILDID"), Value: aws.String(config["buildId"].(string))},
		{Name: aws.String("SD_NO_HAB"), Value: aws.String(strconv.FormatBool(true))},
	}

	log.Printf("Project Arn%q", projectArn)

	if launcherUpdate {
		log.Printf("Starting batch build for project %q", project)
		buildBatchInput := &codebuild.StartBuildBatchInput{
			EnvironmentVariablesOverride: envVars,
			ProjectName:                  aws.String(project),
			ServiceRoleOverride:          aws.String(config["roleArn"].(string)),
			ArtifactsOverride:            initArtifact,
			BuildBatchConfigOverride: &codebuild.ProjectBuildBatchConfig{
				CombineArtifacts: aws.Bool(false),
				Restrictions:     &codebuild.BatchRestrictions{MaximumBuildsAllowed: aws.Int64(2)},
				ServiceRole:      aws.String(config["roleArn"].(string)),
				TimeoutInMins:    aws.Int64(buildTimeout),
			},
			BuildspecOverride:  aws.String(batchBuildSpec),
			SourceTypeOverride: aws.String("NO_SOURCE"),
		}
		if config["logsEnabled"].(bool) {
			buildBatchInput.LogsConfigOverride = &codebuild.LogsConfig{
				CloudWatchLogs: &codebuild.CloudWatchLogsConfig{
					Status: aws.String("ENABLED"),
				},
			}
		}

		// starts a batch build with launcher->build else starts only the build
		_, err = e.svcCBClient.StartBuildBatch(buildBatchInput)
		if err != nil {
			return "", fmt.Errorf("Got error building project: %v", err)
		}
	} else {
		log.Printf("Starting single build for project %q", project)
		// Start single build
		buildInput := &codebuild.StartBuildInput{
			EnvironmentVariablesOverride: envVars,
			ProjectName:                  aws.String(project),
			ServiceRoleOverride:          aws.String(config["roleArn"].(string)),
		}
		if config["logsEnabled"].(bool) {
			buildInput.LogsConfigOverride = &codebuild.LogsConfig{
				CloudWatchLogs: &codebuild.CloudWatchLogsConfig{
					Status: aws.String("ENABLED"),
				},
			}
		}
		_, err = e.svcCBClient.StartBuild(buildInput)
		if err != nil {
			return "", fmt.Errorf("Got error building project: %v", err)
		}
	}

	log.Printf("Started build for project %q", project)

	return projectArn, nil
}

// Stop a build and delete build project
func (e *awsServerless) Stop(config map[string]interface{}) (err error) {
	launcherUpdate := e.checkLauncherUpdate(config["launcherVersion"].(string))
	project := config["jobName"].(string) + "-" + config["jobId"].(string)
	if config["prune "].(bool) {
		deleteProjectResponse, err := e.svcCBClient.DeleteProject(&codebuild.DeleteProjectInput{
			Name: aws.String(project),
		})
		if err != nil {
			return fmt.Errorf("Got error deleting project: %v", err)
		}
		log.Printf("Deleted project %q", *deleteProjectResponse)

		return nil
	}
	if launcherUpdate {
		buildBatchesResponse, _ := e.svcCBClient.ListBuildBatchesForProject(&codebuild.ListBuildBatchesForProjectInput{
			MaxResults:  aws.Int64(5),
			ProjectName: aws.String(project),
			SortOrder:   aws.String("DESCENDING"),
		})
		log.Printf("Build ids for project %q: %v", project, buildBatchesResponse.Ids)
		if len(buildBatchesResponse.Ids) > 0 {
			batchBuildResp, _ := e.svcCBClient.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
				Ids: []*string{
					buildBatchesResponse.Ids[0],
				},
			})
			if string(*batchBuildResp.Builds[0].BuildStatus) == "IN_PROGRESS" {
				stopBuildBatchResponse, err := e.svcCBClient.StopBuildBatch(&codebuild.StopBuildBatchInput{
					Id: buildBatchesResponse.Ids[0],
				})
				if err != nil {
					return fmt.Errorf("Got error stopping build: %v", err)
				}
				log.Printf("Stopped build batch %q for project %v", project, *stopBuildBatchResponse.BuildBatch.BuildBatchNumber)
			}
		}
	} else {
		buildsResponse, _ := e.svcCBClient.ListBuildsForProject(&codebuild.ListBuildsForProjectInput{
			ProjectName: aws.String(project),
			SortOrder:   aws.String("DESCENDING"),
		})

		log.Printf("Build ids for project %q: %v", project, buildsResponse.Ids)
		if len(buildsResponse.Ids) > 0 {
			buildResp, _ := e.svcCBClient.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
				Ids: []*string{
					buildsResponse.Ids[0],
				},
			})
			if string(*buildResp.Builds[0].BuildStatus) == "IN_PROGRESS" {
				stopBuildResponse, err := e.svcCBClient.StopBuild(&codebuild.StopBuildInput{
					Id: buildsResponse.Ids[0],
				})
				if err != nil {
					return fmt.Errorf("Got error stopping build: %v", err)
				}
				log.Printf("Stopped build %q for project %v", project, *stopBuildResponse.Build.BuildNumber)
			}
		}
	}

	return nil
}

//Returns the name of executor
func (e *awsServerless) Name() string {
	return e.name
}

func New() *awsServerless {
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION"))},
	)
	// Create CodeBuild & S3 service client
	svcCB := codebuild.New(sess)
	svcS3 := s3.New(sess)
	return &awsServerless{
		svcCBClient: svcCB,
		svcS3Client: svcS3,
		name:        "sls",
	}
}
