package sls

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/codebuild"
	"github.com/aws/aws-sdk-go/service/codebuild/codebuildiface"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	testNamespace       = "sd-builds"
	testJobName         = "deploy"
	testJobID           = "123"
	testBuildID         = 1234
	testBucket          = "test-bucket-123-usw2"
	testLauncherVersion = "v101"
	testLauncherUpdate  = false
)

func getTestConfig() map[string]interface{} {
	configObj := `{
		"jobName": "deploy",
		"jobId": 123,
		"namespace": "sd-builds",
		"buildId": 1234,
		"container": "node:12",
		"privilegedMode": false,
		"pipelineId": "12345",
		"token": "abc",
		"storeUri": "store.uri",
		"apiUri": "api.uri",
		"uiUri": "ui.uri",
		"buildTimeout": 20,
		"isPR": false,
		"provider": {
			"role": "role:123",
			"region": "us-west-2",
			"buildRegion": "us-west-2",
			"vpc": {
				"vpcId": "vpc-12345",
				"securityGroupIds": [
					"sg-123"
				],
				"subnetIds": [
					"subnet-1111",
					"subnet-2222",
					"subnet-3333"
				]
			},
			"imagePullCredentialsType": "SERVICE_ROLE",
			"environmentType": "LINUX_CONTAINER",
			"computeType": "BUILD_GENERAL1_SMALL",
			"launcherComputeType": "BUILD_GENERAL1_SMALL",
			"logsEnabled": false,
			"prune": false,
			"dlc": false,
			"launcherImage": "launcher:v101",
			"launcherVersion": "v101",
			"queuedTimeout": 5,
			"privilegedMode": false,
			"executorLogs": false,
			"debugSession": false
		}
	}`

	decoder := json.NewDecoder(strings.NewReader(configObj))
	decoder.UseNumber()
	var testConfig map[string]interface{}
	if err := decoder.Decode(&testConfig); err != nil {
		log.Fatal(err)
	}
	testConfig["bucket"] = testBucket

	return testConfig
}

type mockCodeBuildClient struct {
	codebuildiface.CodeBuildAPI
	mock.Mock
}
type mockS3Client struct {
	s3iface.S3API
	mock.Mock
}

func (m *mockCodeBuildClient) BatchGetProjects(input *codebuild.BatchGetProjectsInput) (*codebuild.BatchGetProjectsOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.BatchGetProjectsOutput), args.Error(1)
}
func (m *mockCodeBuildClient) CreateProject(input *codebuild.CreateProjectInput) (*codebuild.CreateProjectOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.CreateProjectOutput), args.Error(1)
}
func (m *mockCodeBuildClient) UpdateProject(input *codebuild.UpdateProjectInput) (*codebuild.UpdateProjectOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.UpdateProjectOutput), args.Error(1)
}
func (m *mockCodeBuildClient) StartBuild(input *codebuild.StartBuildInput) (*codebuild.StartBuildOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.StartBuildOutput), args.Error(1)
}
func (m *mockCodeBuildClient) StartBuildBatch(input *codebuild.StartBuildBatchInput) (*codebuild.StartBuildBatchOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.StartBuildBatchOutput), args.Error(1)
}
func (m *mockCodeBuildClient) DeleteProject(input *codebuild.DeleteProjectInput) (*codebuild.DeleteProjectOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.DeleteProjectOutput), args.Error(1)
}
func (m *mockCodeBuildClient) ListBuildsForProject(input *codebuild.ListBuildsForProjectInput) (*codebuild.ListBuildsForProjectOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.ListBuildsForProjectOutput), args.Error(1)
}
func (m *mockCodeBuildClient) BatchGetBuilds(input *codebuild.BatchGetBuildsInput) (*codebuild.BatchGetBuildsOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.BatchGetBuildsOutput), args.Error(1)
}
func (m *mockCodeBuildClient) StopBuild(input *codebuild.StopBuildInput) (*codebuild.StopBuildOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.StopBuildOutput), args.Error(1)
}
func (m *mockCodeBuildClient) StopBuildBatch(input *codebuild.StopBuildBatchInput) (*codebuild.StopBuildBatchOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*codebuild.StopBuildBatchOutput), args.Error(1)
}
func (m *mockS3Client) ListObjectsV2(input *s3.ListObjectsV2Input) (*s3.ListObjectsV2Output, error) {
	args := m.Called(input)
	return args.Get(0).(*s3.ListObjectsV2Output), args.Error(1)
}

func setup() (*awsAPI, *mockCodeBuildClient, *mockS3Client) {
	mockS3API := new(mockS3Client)
	mockCBAPI := new(mockCodeBuildClient)
	mockServiceClient := &awsAPI{
		s3: mockS3API,
		cb: mockCBAPI,
	}

	return mockServiceClient, mockCBAPI, mockS3API
}

func TestCheckLauncherUpdate(t *testing.T) {
	mockServiceClient, _, mockS3API := setup()
	sourceID := "sdinit-" + testLauncherVersion

	os.Setenv("SD_SLS_BUILD_BUCKET", testBucket)
	defer os.Unsetenv("SD_SLS_BUILD_BUCKET")

	testCases := []struct {
		message        string
		expectedInput  string
		expectedOutput bool
		expectedError  error
	}{
		{
			message:        "launcher update false",
			expectedInput:  "v101",
			expectedOutput: false,
			expectedError:  nil,
		},
		{
			message:        "launcher update true",
			expectedInput:  "v102",
			expectedOutput: true,
			expectedError:  nil,
		},
	}
	for _, testCase := range testCases {
		mockS3API.On("ListObjectsV2", &s3.ListObjectsV2Input{Bucket: aws.String(testBucket)}).Return(&s3.ListObjectsV2Output{Contents: []*s3.Object{{Key: aws.String(sourceID)}}}, testCase.expectedError)
		got := checkLauncherUpdate(mockServiceClient, testCase.expectedInput, testBucket)
		assert.IsType(t, testCase.expectedOutput, got)
		assert.Equal(t, testCase.expectedOutput, got)

	}
}
func TestCheckLauncherUpdateWithFailure(t *testing.T) {
	mockServiceClient, _, mockS3API := setup()
	sourceID := "sdinit-" + testLauncherVersion

	os.Setenv("SD_SLS_BUILD_BUCKET", testBucket)
	defer os.Unsetenv("SD_SLS_BUILD_BUCKET")

	errTestCase := struct {
		message        string
		expectedInput  string
		expectedOutput bool
		expectedError  error
	}{
		message:        "launcher update true when error",
		expectedInput:  "v101",
		expectedOutput: true,
		expectedError:  errors.New("Error getting object"),
	}
	mockS3API.On("ListObjectsV2", &s3.ListObjectsV2Input{Bucket: aws.String(testBucket)}).Return(&s3.ListObjectsV2Output{Contents: []*s3.Object{{Key: aws.String(sourceID)}}}, errTestCase.expectedError)
	got := checkLauncherUpdate(mockServiceClient, errTestCase.expectedInput, testBucket)
	assert.IsType(t, errTestCase.expectedOutput, got)
	assert.Equal(t, errTestCase.expectedOutput, got)
}

// func TestGetBucketName(t *testing.T) {

// }

func TestStart(t *testing.T) {
	projectName := testJobName + "-" + testJobID
	projectArn := "arn:aws:codebuild:project//" + projectName
	testConfig := getTestConfig()
	testConfigWithLauncherUpdate := getTestConfig()
	provider := testConfigWithLauncherUpdate["provider"].(map[string]interface{})
	provider["launcherImage"] = "launcher:v102"
	provider["launcherVersion"] = "v102"
	testConfigWithLauncherUpdate["provider"] = provider

	//create project
	testCases := []struct {
		message              string
		expectedInput        map[string]interface{}
		expectedOutput       string
		expectedError        error
		batchGetError        error
		createProjectError   error
		startBuildError      error
		startBuildBatchError error
		updateProjectError   error
		launcherUpdate       bool
	}{
		{
			message:            "create project if it does not exist and start build",
			expectedInput:      testConfig,
			expectedOutput:     projectArn,
			expectedError:      nil,
			batchGetError:      nil,
			createProjectError: nil,
			startBuildError:    nil,
		},
		{
			message:            "create project fails when batch get fails but project exists",
			expectedInput:      testConfig,
			expectedOutput:     "",
			expectedError:      errors.New("Error-CreateProject: Project already exits"),
			batchGetError:      errors.New("Batch get project failed"),
			createProjectError: errors.New("Project already exits"),
			startBuildError:    nil,
		},
		{
			message:            "create project fails with error",
			expectedInput:      testConfig,
			expectedOutput:     "",
			expectedError:      errors.New("Error-CreateProject: Role does not exist"),
			batchGetError:      nil,
			createProjectError: errors.New("Role does not exist"),
			startBuildError:    nil,
		},
		{
			message:            "start build fails",
			expectedInput:      testConfig,
			expectedOutput:     "",
			expectedError:      errors.New("Got error building project: start build failed"),
			batchGetError:      nil,
			createProjectError: nil,
			startBuildError:    errors.New("start build failed"),
		},
		{
			message:              "create project succeeds with launcher update and batch build ",
			expectedInput:        testConfigWithLauncherUpdate,
			expectedOutput:       projectArn,
			expectedError:        nil,
			batchGetError:        nil,
			launcherUpdate:       true,
			createProjectError:   nil,
			startBuildBatchError: nil,
		},
		{
			message:              "start build batch fails",
			expectedInput:        testConfigWithLauncherUpdate,
			expectedOutput:       "",
			expectedError:        errors.New("Got error building project: start build batch failed"),
			batchGetError:        nil,
			createProjectError:   nil,
			launcherUpdate:       true,
			startBuildBatchError: errors.New("start build batch failed"),
		},
	}

	os.Setenv("SD_SLS_BUILD_BUCKET", testBucket)
	os.Setenv("SD_SLS_BUILD_ENCRYPTION_KEY_ALIAS", "alias/testKey")
	defer os.Unsetenv("SD_SLS_BUILD_BUCKET")
	defer os.Unsetenv("SD_SLS_BUILD_ENCRYPTION_KEY_ALIAS")

	for _, testCase := range testCases {
		provider := testCase.expectedInput["provider"].(map[string]interface{})
		launcherVersion := provider["launcherVersion"].(string)
		createRequest, batchBuildSpec := getRequestObject(projectName, launcherVersion, testCase.launcherUpdate, testCase.expectedInput)
		sourceID := "sdinit-" + testLauncherVersion
		var names []*string
		names = append(names, aws.String(projectName))
		envVars := getEnvVars(testCase.expectedInput)
		startBuildRequest := &codebuild.StartBuildInput{
			EnvironmentVariablesOverride: envVars,
			ProjectName:                  aws.String(projectName),
			ServiceRoleOverride:          aws.String(provider["role"].(string)),
		}
		buildBatchInput := getStartBuildBatchInput(envVars, projectName, testConfigWithLauncherUpdate, batchBuildSpec)

		mockServiceClient, mockCBAPI, mockS3API := setup()
		mockS3API.On("ListObjectsV2", &s3.ListObjectsV2Input{Bucket: aws.String(testBucket)}).Return(&s3.ListObjectsV2Output{Contents: []*s3.Object{{Key: aws.String(sourceID)}}}, nil)
		mockCBAPI.On("BatchGetProjects", &codebuild.BatchGetProjectsInput{Names: names}).Return(&codebuild.BatchGetProjectsOutput{}, testCase.batchGetError)
		mockCBAPI.On("CreateProject", createRequest).Return(&codebuild.CreateProjectOutput{Project: &codebuild.Project{Arn: aws.String(projectArn)}}, testCase.createProjectError)
		mockCBAPI.On("StartBuild", startBuildRequest).Return(&codebuild.StartBuildOutput{}, testCase.startBuildError)
		mockCBAPI.On("StartBuildBatch", buildBatchInput).Return(&codebuild.StartBuildBatchOutput{}, testCase.startBuildBatchError)

		executor := &AwsServerless{
			serviceClient: mockServiceClient,
		}

		got, err := executor.Start(testCase.expectedInput)

		assert.IsType(t, testCase.expectedOutput, got, testCase.message)
		assert.Equal(t, testCase.expectedOutput, got, testCase.message)
		assert.IsType(t, testCase.expectedError, err, testCase.message)
		assert.Equal(t, testCase.expectedError, err, testCase.message)
	}
}

func TestStartWhenProjectExists(t *testing.T) {
	projectName := testJobName + "-" + testJobID
	projectArn := "arn:aws:codebuild:project//" + projectName
	testConfig := getTestConfig()
	testConfigWithLauncherUpdate := getTestConfig()
	provider := testConfigWithLauncherUpdate["provider"].(map[string]interface{})
	provider["launcherImage"] = "launcher:v102"
	provider["launcherVersion"] = "v102"
	testConfigWithLauncherUpdate["provider"] = provider

	//update project
	testCases := []struct {
		message              string
		expectedInput        map[string]interface{}
		expectedOutput       string
		expectedError        error
		batchGetError        error
		startBuildError      error
		startBuildBatchError error
		updateProjectError   error
		launcherUpdate       bool
	}{
		{
			message:            "Update project if it does not exist and start build",
			expectedInput:      testConfig,
			expectedOutput:     projectArn,
			expectedError:      nil,
			batchGetError:      nil,
			updateProjectError: nil,
			startBuildError:    nil,
		},
		{
			message:            "Update project fails when batch get fails but project does not exists",
			expectedInput:      testConfig,
			expectedOutput:     "",
			expectedError:      errors.New("Error-UpdateProject: Project does not exist"),
			batchGetError:      errors.New("Batch get project failed"),
			updateProjectError: errors.New("Project does not exist"),
			startBuildError:    nil,
		},
		{
			message:            "Update project fails with error",
			expectedInput:      testConfig,
			expectedOutput:     "",
			expectedError:      errors.New("Error-UpdateProject: Role does not exist"),
			batchGetError:      nil,
			updateProjectError: errors.New("Role does not exist"),
			startBuildError:    nil,
		},
		{
			message:            "start build fails",
			expectedInput:      testConfig,
			expectedOutput:     "",
			expectedError:      errors.New("Got error building project: start build failed"),
			batchGetError:      nil,
			updateProjectError: nil,
			startBuildError:    errors.New("start build failed"),
		},
		{
			message:              "update project succeeds with launcher update and batch build ",
			expectedInput:        testConfigWithLauncherUpdate,
			expectedOutput:       projectArn,
			expectedError:        nil,
			batchGetError:        nil,
			launcherUpdate:       true,
			updateProjectError:   nil,
			startBuildBatchError: nil,
		},
		{
			message:              "start build batch fails",
			expectedInput:        testConfigWithLauncherUpdate,
			expectedOutput:       "",
			expectedError:        errors.New("Got error building project: start build batch failed"),
			batchGetError:        nil,
			updateProjectError:   nil,
			launcherUpdate:       true,
			startBuildBatchError: errors.New("start build batch failed"),
		},
	}

	os.Setenv("SD_SLS_BUILD_BUCKET", testBucket)
	defer os.Unsetenv("SD_SLS_BUILD_BUCKET")

	for _, testCase := range testCases {
		provider := testCase.expectedInput["provider"].(map[string]interface{})
		launcherVersion := provider["launcherVersion"].(string)
		_request, batchBuildSpec := getRequestObject(projectName, launcherVersion, testCase.launcherUpdate, testCase.expectedInput)
		updateRequest := codebuild.UpdateProjectInput(*_request)
		sourceID := "sdinit-" + testLauncherVersion
		var names []*string
		names = append(names, aws.String(projectName))
		envVars := getEnvVars(testCase.expectedInput)
		startBuildRequest := &codebuild.StartBuildInput{
			EnvironmentVariablesOverride: envVars,
			ProjectName:                  aws.String(projectName),
			ServiceRoleOverride:          aws.String(provider["role"].(string)),
		}
		buildBatchInput := getStartBuildBatchInput(envVars, projectName, testConfigWithLauncherUpdate, batchBuildSpec)

		mockServiceClient, mockCBAPI, mockS3API := setup()
		mockS3API.On("ListObjectsV2", &s3.ListObjectsV2Input{Bucket: aws.String(testBucket)}).Return(&s3.ListObjectsV2Output{Contents: []*s3.Object{{Key: aws.String(sourceID)}}}, nil)
		mockCBAPI.On("BatchGetProjects", &codebuild.BatchGetProjectsInput{Names: names}).Return(&codebuild.BatchGetProjectsOutput{Projects: []*codebuild.Project{{Name: aws.String(projectName)}}}, testCase.batchGetError)
		mockCBAPI.On("UpdateProject", &updateRequest).Return(&codebuild.UpdateProjectOutput{Project: &codebuild.Project{Arn: aws.String(projectArn)}}, testCase.updateProjectError)
		mockCBAPI.On("StartBuild", startBuildRequest).Return(&codebuild.StartBuildOutput{}, testCase.startBuildError)
		mockCBAPI.On("StartBuildBatch", buildBatchInput).Return(&codebuild.StartBuildBatchOutput{}, testCase.startBuildBatchError)

		executor := &AwsServerless{
			serviceClient: mockServiceClient,
		}

		got, err := executor.Start(testCase.expectedInput)

		assert.IsType(t, testCase.expectedOutput, got, testCase.message)
		assert.Equal(t, testCase.expectedOutput, got, testCase.message)
		assert.IsType(t, testCase.expectedError, err, testCase.message)
		assert.Equal(t, testCase.expectedError, err, testCase.message)
	}
}

func TestStop(t *testing.T) {
	projectName := testJobName + "-" + testJobID
	stopConfig := getTestConfig()
	provider := stopConfig["provider"].(map[string]interface{})
	provider["prune"] = true
	stopConfig["provider"] = provider

	testCases := []struct {
		message            string
		expectedInput      map[string]interface{}
		expectedError      error
		deleteProjectError error
	}{
		{
			message:            "Project is deleted successfully",
			expectedInput:      stopConfig,
			expectedError:      nil,
			deleteProjectError: nil,
		},
		{
			message:            "Project is not deleted successfully",
			expectedInput:      stopConfig,
			expectedError:      errors.New("Got error deleting project: Access Denied"),
			deleteProjectError: errors.New("Access Denied"),
		},
	}

	for _, testCase := range testCases {
		mockServiceClient, mockCBAPI, _ := setup()
		mockCBAPI.On("DeleteProject", &codebuild.DeleteProjectInput{Name: aws.String(projectName)}).Return(&codebuild.DeleteProjectOutput{}, testCase.deleteProjectError)

		executor := &AwsServerless{
			serviceClient: mockServiceClient,
		}
		err := executor.Stop(testCase.expectedInput)
		assert.IsType(t, testCase.expectedError, err, testCase.message)
		assert.Equal(t, testCase.expectedError, err, testCase.message)
	}
}
