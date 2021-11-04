package sls

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/aws/aws-sdk-go/service/codebuild/codebuildiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
)

// aws api definition struct
type awsAPI struct {
	cb codebuildiface.CodeBuildAPI
	s3 s3iface.S3API
}

// serverless definition struct
type awsServerless struct {
	serviceClient *awsAPI
	name          string
}

const (
	EXECUTOR = "sls"
	SDINIT   = "sdinit-"
)

// checks if launcher update is required
func checkLauncherUpdate(serviceClient *awsAPI, launcherVersion string) bool {
	bucket := os.Getenv("SD_SLS_BUILD_BUCKET")
	var launcherUpdate bool = true

	listResult, err := serviceClient.s3.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		log.Printf("Failed to get launcher version %v", err.Error())
		return launcherUpdate
	}

	sourceIdentifier := SDINIT + launcherVersion
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

// deletes a build project using codebuild service api
func deleteProject(serviceClient *awsAPI, project string) error {
	deleteProjectResponse, err := serviceClient.cb.DeleteProject(&codebuild.DeleteProjectInput{
		Name: aws.String(project),
	})
	if err != nil {
		return fmt.Errorf("Got error deleting project: %v", err)
	}
	log.Printf("Deleted project %q", *deleteProjectResponse)

	return nil
}

// stops a build using codebuild service api
func stopBuild(serviceClient *awsAPI, project string) error {
	buildsResponse, _ := serviceClient.cb.ListBuildsForProject(&codebuild.ListBuildsForProjectInput{
		ProjectName: aws.String(project),
		SortOrder:   aws.String("DESCENDING"),
	})

	log.Printf("Build ids for project %q: %v", project, buildsResponse.Ids)
	if len(buildsResponse.Ids) > 0 {
		buildResp, _ := serviceClient.cb.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
			Ids: []*string{
				buildsResponse.Ids[0],
			},
		})
		if string(*buildResp.Builds[0].BuildStatus) == "IN_PROGRESS" {
			stopBuildResponse, err := serviceClient.cb.StopBuild(&codebuild.StopBuildInput{
				Id: buildsResponse.Ids[0],
			})
			if err != nil {
				return fmt.Errorf("Got error stopping build: %v", err)
			}
			log.Printf("Stopped build %q for project %v", project, *stopBuildResponse.Build.BuildNumber)
		}
	}
	return nil
}

//stops builds running in batch using codebuild service api
func stopBuildBatch(serviceClient *awsAPI, project string) error {
	buildBatchesResponse, _ := serviceClient.cb.ListBuildBatchesForProject(&codebuild.ListBuildBatchesForProjectInput{
		MaxResults:  aws.Int64(5),
		ProjectName: aws.String(project),
		SortOrder:   aws.String("DESCENDING"),
	})
	log.Printf("Build ids for project %q: %v", project, buildBatchesResponse.Ids)
	if len(buildBatchesResponse.Ids) > 0 {
		batchBuildResp, _ := serviceClient.cb.BatchGetBuilds(&codebuild.BatchGetBuildsInput{
			Ids: []*string{
				buildBatchesResponse.Ids[0],
			},
		})
		if string(*batchBuildResp.Builds[0].BuildStatus) == "IN_PROGRESS" {
			stopBuildBatchResponse, err := serviceClient.cb.StopBuildBatch(&codebuild.StopBuildBatchInput{
				Id: buildBatchesResponse.Ids[0],
			})
			if err != nil {
				return fmt.Errorf("Got error stopping build: %v", err)
			}
			log.Printf("Stopped build batch %q for project %v", project, *stopBuildBatchResponse.BuildBatch.BuildBatchNumber)
		}
	}
	return nil
}

// gets the formatted build spec files for codebuild project
func getBuildSpec(config map[string]interface{}) (string, string) {
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

	return batchBuildSpec, singleBuildSpec
}

// starts a build using codebuild service api
func startBuild(project string, envVars []*codebuild.EnvironmentVariable, config map[string]interface{}, serviceClient *awsAPI) error {
	log.Printf("Starting single build for project %q", project)

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
	_, err := serviceClient.cb.StartBuild(buildInput)
	return err
}

// starts builds in batch using codebuild service api
func startBuildBatch(project string, envVars []*codebuild.EnvironmentVariable, config map[string]interface{}, batchBuildSpec string, serviceClient *awsAPI) error {
	log.Printf("Starting batch build for project %q", project)

	buildBatchInput := getStartBuildBatchInput(envVars, project, config, batchBuildSpec)
	_, err := serviceClient.cb.StartBuildBatch(buildBatchInput)

	return err
}

// gets the input required for running a build batch
func getStartBuildBatchInput(envVars []*codebuild.EnvironmentVariable, project string, config map[string]interface{}, batchBuildSpec string) *codebuild.StartBuildBatchInput {
	buildBatchInput := &codebuild.StartBuildBatchInput{
		EnvironmentVariablesOverride: envVars,
		ProjectName:                  aws.String(project),
		ServiceRoleOverride:          aws.String(config["roleArn"].(string)),
		ArtifactsOverride: &codebuild.ProjectArtifacts{
			EncryptionDisabled:   aws.Bool(false),
			Location:             aws.String(os.Getenv("SD_SLS_BUILD_BUCKET")),
			OverrideArtifactName: aws.Bool(false),
			Packaging:            aws.String("ZIP"),
			Type:                 aws.String("S3"),
			Name:                 aws.String(config["launcherVersion"].(string)),
		},
		BuildBatchConfigOverride: &codebuild.ProjectBuildBatchConfig{
			CombineArtifacts: aws.Bool(false),
			Restrictions:     &codebuild.BatchRestrictions{MaximumBuildsAllowed: aws.Int64(2)},
			ServiceRole:      aws.String(config["roleArn"].(string)),
			TimeoutInMins:    aws.Int64(int64(config["buildTimeout"].(int))),
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
	return buildBatchInput
}

// gets the requests object for create project
func getRequestObject(project string, launcherVersion string, launcherUpdate bool, config map[string]interface{}) (*codebuild.CreateProjectInput, string) {
	batchBuildSpec, singleBuildSpec := getBuildSpec(config)
	sourceIdentifier := SDINIT + launcherVersion

	var securityGroupIds []*string
	for _, sg := range config["securityGroupIds"].([]string) {
		securityGroupIds = append(securityGroupIds, aws.String(sg))
	}

	var subnets []*string
	for _, sn := range config["subnets"].([]string) {
		subnets = append(subnets, aws.String(sn))
	}

	queuedTimeout := int64(config["queuedTimeout"].(int))
	buildTimeout := int64(config["buildTimeout"].(int))

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
	if config["dlc"].(bool) {
		createRequest.Cache = &codebuild.ProjectCache{
			Location: new(string),
			Modes:    []*string{aws.String("LOCAL_DOCKER_LAYER_CACHE")},
			Type:     aws.String("LOCAL"),
		}

		createRequest.Environment.PrivilegedMode = aws.Bool(true)
	}
	return createRequest, batchBuildSpec
}

// gets the environment variables object
func getEnvVars(config map[string]interface{}) []*codebuild.EnvironmentVariable {
	return []*codebuild.EnvironmentVariable{
		{Name: aws.String("TOKEN"), Value: aws.String(config["token"].(string))},
		{Name: aws.String("API"), Value: aws.String(config["apiUri"].(string))},
		{Name: aws.String("STORE"), Value: aws.String(config["storeUri"].(string))},
		{Name: aws.String("UI"), Value: aws.String(config["uiUri"].(string))},
		{Name: aws.String("TIMEOUT"), Value: aws.String(strconv.Itoa(config["buildTimeout"].(int)))},
		{Name: aws.String("SDBUILDID"), Value: aws.String(strconv.Itoa(config["buildId"].(int)))},
		{Name: aws.String("SD_NO_HAB"), Value: aws.String(strconv.FormatBool(true))},
	}
}

// Create a codebuild project and Start a build
func (e *awsServerless) Start(config map[string]interface{}) (string, error) {
	launcherVersion := config["launcherVersion"].(string)
	launcherUpdate := checkLauncherUpdate(e.serviceClient, launcherVersion)

	project := config["jobName"].(string) + "-" + config["jobId"].(string)

	var names []*string
	names = append(names, aws.String(project))
	batchResult, err := e.serviceClient.cb.BatchGetProjects(&codebuild.BatchGetProjectsInput{
		Names: names,
	})

	if err != nil {
		log.Printf("Error-BatchGetProjects: %v, creating project", err)
	}

	createRequest, batchBuildSpec := getRequestObject(project, launcherVersion, launcherUpdate, config)

	var projectArn string

	if batchResult == nil || len(batchResult.Projects) == 0 {
		log.Printf("Project does not exist, creating project")
		createResult, err := e.serviceClient.cb.CreateProject(createRequest)
		if err != nil {
			return "", fmt.Errorf("Error-CreateProject: %v", err)
		}
		log.Printf("Project created: %v", createResult.Project.Arn)
		projectArn = *createResult.Project.Arn
	} else {
		log.Printf("Project already exists, updating project")
		updateRequest := codebuild.UpdateProjectInput(*createRequest)
		updateResult, err := e.serviceClient.cb.UpdateProject(&updateRequest)
		if err != nil {
			return "", fmt.Errorf("Error-UpdateProject: %v", err)
		}
		log.Printf("Project updated: %v", updateResult.Project.Arn)
		projectArn = *updateResult.Project.Arn
	}

	log.Printf("Project Arn%q", projectArn)

	envVars := getEnvVars(config)

	if launcherUpdate {
		// starts a batch build with launcher->build else starts only the build
		err = startBuildBatch(project, envVars, config, batchBuildSpec, e.serviceClient)
	} else {
		// Start single build
		err = startBuild(project, envVars, config, e.serviceClient)
	}

	if err != nil {
		return "", fmt.Errorf("Got error building project: %v", err)
	}

	log.Printf("Started build for project %q", project)

	return projectArn, nil
}

// Stop a build and delete build project
func (e *awsServerless) Stop(config map[string]interface{}) (err error) {
	project := config["jobName"].(string) + "-" + config["jobId"].(string)

	if config["prune"].(bool) {
		return deleteProject(e.serviceClient, project)
	}
	if checkLauncherUpdate(e.serviceClient, config["launcherVersion"].(string)) {
		return stopBuildBatch(e.serviceClient, project)
	} else {
		return stopBuild(e.serviceClient, project)
	}
}

// Name returns the name of executor
func (e *awsServerless) Name() string {
	return e.name
}

// New returns a new instance of executor and service client
func New() *awsServerless {
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION"))},
	)
	// Create CodeBuild & S3 service client
	svcClient := &awsAPI{
		s3: s3.New(sess),
		cb: codebuild.New(sess),
	}

	return &awsServerless{
		name:          EXECUTOR,
		serviceClient: svcClient,
	}
}
