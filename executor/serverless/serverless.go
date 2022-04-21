package sls

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

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

// AwsServerless definition struct
type AwsServerless struct {
	serviceClient *awsAPI
	name          string
}

const (
	executorName = "sls"
	sdInitPrefix = "sdinit-"
)

// awsRegionMap for region short names
var awsRegionMap = map[string]string{
	"north":     "n",
	"west":      "w",
	"northeast": "ne",
	"east":      "e",
	"south":     "s",
	"central":   "c",
	"southeast": "se",
}

// gets the region short name
func getRegionShortName(region string) string {
	items := strings.Split(region, "-")
	shortRegion := strings.Join([]string{items[0], awsRegionMap[items[1]], items[2]}, "")
	return shortRegion
}

// gets the bucket name in case of cross region deployments
func getBucketName(region string, buildRegion string) string {
	bucket := os.Getenv("SD_SLS_BUILD_BUCKET")
	if buildRegion == "" || region == buildRegion {
		return bucket
	}
	bucketName := strings.Replace(bucket, getRegionShortName(region), getRegionShortName(buildRegion), 1)

	log.Printf("Regional Bucket Name: %v", bucketName)

	return bucketName
}

// checks if launcher update is required
func checkLauncherUpdate(serviceClient *awsAPI, launcherVersion string, bucket string) bool {
	var launcherUpdate bool = true

	listResult, err := serviceClient.s3.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		log.Printf("Failed to get launcher version %v", err.Error())
		return launcherUpdate
	}

	sourceIdentifier := sdInitPrefix + launcherVersion
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
func getBuildSpec(provider map[string]interface{}) (string, string) {
	mainBuildspec := fmt.Sprintf("version: 0.2\\nphases:\\n  install:\\n    commands:\\n      - mkdir /opt/sd && cp -r $CODEBUILD_SRC_DIR_sdinit_sdinit/opt/sd/* /opt/sd/\\n  build:\\n    commands:\\n      - /opt/sd/launcher_entrypoint.sh /opt/sd/run.sh $TOKEN $API $STORE $TIMEOUT $SDBUILDID $UI")
	batchBuildSpec := fmt.Sprintf("version: 0.2\nbatch:\n  fast-fail: false\n  build-graph:\n    - identifier: sdinit\n      env:\n        type: %v\n        image: %v\n        compute-type: %v\n        privileged-mode: false\n      ignore-failure: false\n    - identifier: main\n      buildspec: \"%v\"\n      depend-on:\n        - sdinit\nartifacts:\n  base-directory: /opt\n  files:  \n    - '/opt/**/*'",
		provider["launcherEnvironmentType"].(string), provider["launcherImage"].(string), provider["launcherComputeType"].(string), mainBuildspec)
	singleBuildSpec := fmt.Sprintf("version: 0.2\nphases:\n  install:\n    commands:\n       - mkdir /opt/sd && cp -r $CODEBUILD_SRC_DIR/opt/sd/* /opt/sd/\n  build:\n    commands:\n       - /opt/sd/launcher_entrypoint.sh /opt/sd/run.sh $TOKEN $API $STORE $TIMEOUT $SDBUILDID $UI\n")

	return batchBuildSpec, singleBuildSpec
}

// starts a build using codebuild service api
func startBuild(project string, envVars []*codebuild.EnvironmentVariable, provider map[string]interface{}, serviceClient *awsAPI) error {
	log.Printf("Starting single build for project %q", project)

	buildInput := &codebuild.StartBuildInput{
		EnvironmentVariablesOverride: envVars,
		ProjectName:                  aws.String(project),
		ServiceRoleOverride:          aws.String(provider["role"].(string)),
	}
	if provider["executorLogs"].(bool) {
		buildInput.LogsConfigOverride = &codebuild.LogsConfig{
			CloudWatchLogs: &codebuild.CloudWatchLogsConfig{
				Status: aws.String("ENABLED"),
			},
		}
	}
	if provider["debugSession"].(bool) {
		buildInput.DebugSessionEnabled = aws.Bool(true)
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
	provider := config["provider"].(map[string]interface{})
	buildTimeout, _ := config["buildTimeout"].(json.Number).Int64()
	buildBatchInput := &codebuild.StartBuildBatchInput{
		EnvironmentVariablesOverride: envVars,
		ProjectName:                  aws.String(project),
		ServiceRoleOverride:          aws.String(provider["role"].(string)),
		ArtifactsOverride: &codebuild.ProjectArtifacts{
			EncryptionDisabled:   aws.Bool(false),
			Location:             aws.String(config["bucket"].(string)),
			OverrideArtifactName: aws.Bool(false),
			Packaging:            aws.String("ZIP"),
			Type:                 aws.String("S3"),
			Name:                 aws.String(provider["launcherVersion"].(string)),
		},
		BuildBatchConfigOverride: &codebuild.ProjectBuildBatchConfig{
			CombineArtifacts: aws.Bool(false),
			Restrictions:     &codebuild.BatchRestrictions{MaximumBuildsAllowed: aws.Int64(2)},
			ServiceRole:      aws.String(provider["role"].(string)),
			TimeoutInMins:    aws.Int64(buildTimeout),
		},
		BuildspecOverride:  aws.String(batchBuildSpec),
		SourceTypeOverride: aws.String("NO_SOURCE"),
	}
	if provider["executorLogs"].(bool) {
		buildBatchInput.LogsConfigOverride = &codebuild.LogsConfig{
			CloudWatchLogs: &codebuild.CloudWatchLogsConfig{
				Status: aws.String("ENABLED"),
			},
		}
	}
	if provider["debugSession"].(bool) {
		buildBatchInput.DebugSessionEnabled = aws.Bool(true)
	}

	return buildBatchInput
}

// gets the requests object for create project
func getRequestObject(project string, launcherVersion string, launcherUpdate bool, config map[string]interface{}) (*codebuild.CreateProjectInput, string) {
	provider := config["provider"].(map[string]interface{})

	if provider["environmentType"].(string) == "ARM_CONTAINER" {
		provider["launcherEnvironmentType"] = "ARM_CONTAINER"
	}

	batchBuildSpec, singleBuildSpec := getBuildSpec(provider)
	sourceIdentifier := sdInitPrefix + launcherVersion

	vpc := provider["vpc"].(map[string]interface{})

	var securityGroupIds []*string
	for _, sg := range vpc["securityGroupIds"].([]interface{}) {
		securityGroupIds = append(securityGroupIds, aws.String(sg.(string)))
	}

	var subnets []*string
	for _, sn := range vpc["subnetIds"].([]interface{}) {
		subnets = append(subnets, aws.String(sn.(string)))
	}

	queuedTimeout, _ := provider["queuedTimeout"].(json.Number).Int64()
	buildTimeout, _ := config["buildTimeout"].(json.Number).Int64()

	// set image pull credential type
	if strings.HasPrefix(config["container"].(string), "aws/codebuild/") {
		imagePullCredentialsType := "CODEBUILD"
		provider["imagePullCredentialsType"] = imagePullCredentialsType
	}

	createRequest := &codebuild.CreateProjectInput{
		Artifacts: &codebuild.ProjectArtifacts{
			Type: aws.String("NO_ARTIFACTS"),
		},
		Name: aws.String(project),
		Source: &codebuild.ProjectSource{
			Buildspec: aws.String(singleBuildSpec),
			Location:  aws.String(config["bucket"].(string) + "/" + sourceIdentifier),
			Type:      aws.String("S3"),
		},
		QueuedTimeoutInMinutes: aws.Int64(queuedTimeout),
		ServiceRole:            aws.String(provider["role"].(string)),
		TimeoutInMinutes:       aws.Int64(buildTimeout),
		ConcurrentBuildLimit:   aws.Int64(2),
		Environment: &codebuild.ProjectEnvironment{
			ComputeType:              aws.String(provider["computeType"].(string)),
			Image:                    aws.String(config["container"].(string)),
			ImagePullCredentialsType: aws.String(provider["imagePullCredentialsType"].(string)),
			PrivilegedMode:           aws.Bool(provider["privilegedMode"].(bool)),
			Type:                     aws.String(provider["environmentType"].(string)),
		},
		VpcConfig: &codebuild.VpcConfig{
			SecurityGroupIds: securityGroupIds,
			Subnets:          subnets,
			VpcId:            aws.String(vpc["vpcId"].(string)),
		},
		LogsConfig: &codebuild.LogsConfig{
			CloudWatchLogs: &codebuild.CloudWatchLogsConfig{
				Status: aws.String("DISABLED"),
			},
			S3Logs: &codebuild.S3LogsConfig{
				Status: aws.String("DISABLED"),
			},
		},
		EncryptionKey: aws.String(os.Getenv("SD_SLS_BUILD_ENCRYPTION_KEY_ALIAS")),
	}

	if launcherUpdate {
		createRequest.Source = &codebuild.ProjectSource{
			Buildspec: aws.String(batchBuildSpec),
			Type:      aws.String("NO_SOURCE"),
		}
		createRequest.BuildBatchConfig = &codebuild.ProjectBuildBatchConfig{
			CombineArtifacts: aws.Bool(false),
			Restrictions:     &codebuild.BatchRestrictions{MaximumBuildsAllowed: aws.Int64(2)},
			ServiceRole:      aws.String(provider["role"].(string)),
			TimeoutInMins:    aws.Int64(buildTimeout),
		}
	}
	if provider["dlc"].(bool) {
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
	buildTimeout, _ := config["buildTimeout"].(json.Number).Int64()
	buildID, _ := config["buildId"].(json.Number).Int64()
	return []*codebuild.EnvironmentVariable{
		{Name: aws.String("TOKEN"), Value: aws.String(config["token"].(string))},
		{Name: aws.String("API"), Value: aws.String(config["apiUri"].(string))},
		{Name: aws.String("STORE"), Value: aws.String(config["storeUri"].(string))},
		{Name: aws.String("UI"), Value: aws.String(config["uiUri"].(string))},
		{Name: aws.String("TIMEOUT"), Value: aws.String(fmt.Sprint(buildTimeout))},
		{Name: aws.String("SDBUILDID"), Value: aws.String(fmt.Sprint(buildID))},
		{Name: aws.String("SD_HAB_ENABLED"), Value: aws.String(strconv.FormatBool(false))},
		{Name: aws.String("SD_AWS_INTEGRATION"), Value: aws.String(strconv.FormatBool(true))},
	}
}

// gets the formatted project name
func getProjectName(config map[string]interface{}) string {
	var jobName = config["jobName"].(string)
	if config["isPR"].(bool) {
		jobName = strings.Replace(jobName, ":", "-", 1)
	}
	jobID, _ := config["jobId"].(json.Number).Int64()
	projectName := jobName + "-" + fmt.Sprint(jobID)

	log.Printf("Project name: %v", projectName)

	return projectName
}

// Start function of executor creates a codebuild project and starts a build
func (e *AwsServerless) Start(config map[string]interface{}) (string, error) {
	provider := config["provider"].(map[string]interface{})

	launcherVersion := provider["launcherVersion"].(string)
	bucket := getBucketName(provider["region"].(string), provider["buildRegion"].(string))
	// set bucket to config
	config["bucket"] = bucket

	launcherUpdate := checkLauncherUpdate(e.serviceClient, launcherVersion, bucket)

	log.Printf("Launcher Updated: %v", launcherUpdate)

	project := getProjectName(config)

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
		projectArn = *createResult.Project.Arn
	} else {
		log.Printf("Project already exists, updating project")
		updateRequest := codebuild.UpdateProjectInput(*createRequest)
		updateResult, err := e.serviceClient.cb.UpdateProject(&updateRequest)
		if err != nil {
			return "", fmt.Errorf("Error-UpdateProject: %v", err)
		}
		projectArn = *updateResult.Project.Arn
	}

	log.Printf("Project Arn%q", projectArn)

	envVars := getEnvVars(config)

	if launcherUpdate {
		// starts a batch build with launcher->build else starts only the build
		err = startBuildBatch(project, envVars, config, batchBuildSpec, e.serviceClient)
	} else {
		// Start single build
		err = startBuild(project, envVars, provider, e.serviceClient)
	}

	if err != nil {
		return "", fmt.Errorf("Got error building project: %v", err)
	}

	log.Printf("Started build for project %q", project)

	return projectArn, nil
}

// Stop a build and delete build project
func (e *AwsServerless) Stop(config map[string]interface{}) (err error) {
	provider := config["provider"].(map[string]interface{})
	project := getProjectName(config)

	if provider["prune"].(bool) {
		return deleteProject(e.serviceClient, project)
	}

	bucket := getBucketName(provider["region"].(string), provider["buildRegion"].(string))
	// set bucket to config
	config["bucket"] = bucket

	if checkLauncherUpdate(e.serviceClient, provider["launcherVersion"].(string), bucket) {
		return stopBuildBatch(e.serviceClient, project)
	}
	return stopBuild(e.serviceClient, project)
}

// Name returns the name of executor
func (e *AwsServerless) Name() string {
	return e.name
}

// New returns a new instance of executor and service client
func New(region string) *AwsServerless {
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	// Create CodeBuild & S3 service client
	svcClient := &awsAPI{
		s3: s3.New(sess),
		cb: codebuild.New(sess),
	}

	return &AwsServerless{
		name:          executorName,
		serviceClient: svcClient,
	}
}
