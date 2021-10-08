package eks

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/eks/eksiface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	fake "k8s.io/client-go/kubernetes/fake"
)

var (
	clusterName              = "test-cluster-1"
	clusterIDDoesNotExist    = "Lorem Ipsum is simply dummy text"
	expectedClusterOutputArn = "arn:" + clusterName
	testNamespace            = "sd-builds"
	testPrefix               = "beta"
	testBuildId              = 1234
	testConfig               = map[string]interface{}{
		"clusterName":        clusterName,
		"namespace":          testNamespace,
		"prefix":             testPrefix,
		"buildId":            testBuildId,
		"serviceAccountName": "default",
		"container":          "node:12",
		"privilegedMode":     false,
		"cpuLimit":           "2Gi",
		"memoryLimit":        "2Gi",
		"launcherImage":      "launcher:v101",
		"launcherVersion":    "v101",
		"pipelineId":         "12345",
		"token":              "abc",
		"storeUri":           "store.uri",
		"apiUri":             "api.uri",
		"uiUri":              "ui.uri",
		"buildTimeout":       "20",
	}
)

var (
	validCluster        = clusterName
	emptyCluster        = ""
	clusterDoesNotExist = clusterIDDoesNotExist
)

type mockEKS struct {
	eksiface.EKSAPI
	mock.Mock
}

func (m *mockEKS) DescribeCluster(input *eks.DescribeClusterInput) (*eks.DescribeClusterOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*eks.DescribeClusterOutput), args.Error(1)
}

func setup() (*mockEKS, *eksClient) {
	mockEKSClient := new(mockEKS)
	mockEKS := &eksClient{
		service: mockEKSClient,
	}

	return mockEKSClient, mockEKS
}

type awsEKSExecutorMock struct {
	name         string
	eksClient    *mockEKS
	k8sClientSet *fake.Clientset
}

func (e *awsEKSExecutorMock) getToken(cluster_name *string) (string, error) {
	return "token:1234", nil
}

func (e *awsEKSExecutorMock) newClientSet(config map[string]interface{}) (*fake.Clientset, error) {
	kubeclient := fake.NewSimpleClientset()
	return kubeclient, nil
}

func TestNewAWSService(t *testing.T) {
	awsService := newEKSService("us-west-2")
	assert.NotNil(t, awsService.service)
}

func TestDescribeCluster(t *testing.T) {
	testCases := []struct {
		message        string
		clusterName    string
		expectedInput  string
		expectedOutput string
		eksError       error
		expectedError  error
	}{
		{
			message:        "When cluster ID is empty, return error",
			clusterName:    "",
			expectedInput:  emptyCluster,
			expectedOutput: "",
			eksError:       nil,
			expectedError:  errors.New("cluster Name is empty"),
		},
		{
			message:        "When cluster name is valid, return cluster info",
			clusterName:    clusterName,
			expectedInput:  validCluster,
			expectedOutput: expectedClusterOutputArn,
			eksError:       nil,
			expectedError:  nil,
		},
		{
			message:        "when DescribeCluster method fails, return error",
			clusterName:    clusterName,
			expectedInput:  validCluster,
			expectedOutput: "",
			eksError:       errors.New("DescribeCluster method failure"),
			expectedError:  errors.New("DescribeCluster method failure"),
		},
		{
			message:        "when cluster ID does not exist",
			clusterName:    clusterIDDoesNotExist,
			expectedInput:  clusterDoesNotExist,
			expectedOutput: "",
			eksError:       errors.New("cluster does not exist"),
			expectedError:  errors.New("cluster does not exist"),
		},
	}

	for _, testCase := range testCases {
		mockEKSClient, mockEKS := setup()

		mockDescribeClusterInput := &eks.DescribeClusterInput{
			Name: aws.String(testCase.clusterName),
		}
		mockDescribeClusterOutput := &eks.DescribeClusterOutput{Cluster: &eks.Cluster{
			Arn: aws.String("arn:" + testCase.clusterName),
			CertificateAuthority: &eks.Certificate{
				Data: aws.String("somedata"),
			},
			Name:     aws.String(testCase.clusterName),
			Endpoint: aws.String("endpoint://" + testCase.clusterName),
		}}

		mockEKSClient.On("DescribeCluster", mockDescribeClusterInput).Return(mockDescribeClusterOutput, testCase.eksError)
		res, err := mockEKS.describeCluster(testCase.expectedInput)
		got := ""
		if err == nil {
			got = string(*res.Cluster.Arn)
		}

		assert.Equal(t, testCase.expectedOutput, got, testCase.message)
		assert.IsType(t, testCase.expectedError, err, testCase.message)
	}
}
func TestK8sClientSet(t *testing.T) {
	mockEKSClient, mockEKS := setup()
	mockDescribeClusterInput := &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	}
	mockDescribeClusterOutput := &eks.DescribeClusterOutput{Cluster: &eks.Cluster{
		Arn: aws.String("arn:" + clusterName),
		CertificateAuthority: &eks.Certificate{
			Data: aws.String("somedata"),
		},
		Name:     aws.String(clusterName),
		Endpoint: aws.String("endpoint://" + clusterName),
	}}

	mockEKSClient.On("DescribeCluster", mockDescribeClusterInput).Return(mockDescribeClusterOutput, nil)

	executor := &awsExecutorEKS{
		eksClient: mockEKS,
	}
	tests := []struct {
		request   map[string]interface{}
		expect    *kubernetes.Clientset
		expectObj *k8sClientset
		err       error
	}{
		{
			request: map[string]interface{}{
				"clusterName": "",
			},
			expectObj: &k8sClientset{},
			err:       errors.New("Error calling DescribeCluster:cluster name is empty"),
		},
		{
			request: map[string]interface{}{
				"clusterName": clusterName,
			},
			expect: &kubernetes.Clientset{},
			err:    nil,
		},
	}
	for _, test := range tests {
		got, err := executor.newClientSet(test.request)
		if err != nil {
			assert.IsType(t, test.expectObj, got)
		} else {
			assert.IsType(t, test.expect, got.client)
		}
		assert.Equal(t, test.err, err)
		assert.IsType(t, test.err, err)
	}
}

func TestStart(t *testing.T) {
	kubeclient := fake.NewSimpleClientset(&core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pod-1",
			Namespace: testNamespace,
		},
	}, &core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pod-2",
			Namespace: testNamespace,
		},
	}, &core.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ip-12-3-4.example.com",
			Labels: map[string]string{"app": "screwdriver", "tier": "builds"},
		},
		Spec: core.NodeSpec{
			PodCIDR: "10.0.0.0/16",
		},
		Status: core.NodeStatus{},
	})
	executor := &awsExecutorEKS{
		k8sClientset: &k8sClientset{
			client: kubeclient,
		},
	}
	errorConfig := make(map[string]interface{})
	for key, value := range testConfig {
		errorConfig[key] = value
	}
	errorConfig["namespace"] = "default"

	tests := []struct {
		request          map[string]interface{}
		expectedPodCount int
		expectedNode     string
		err              error
	}{
		{
			request:          errorConfig,
			expectedNode:     "ip-12-3-4.example.com",
			expectedPodCount: 0,
			err:              nil,
		},
		{
			request:          testConfig,
			expectedNode:     "ip-12-3-4.example.com",
			err:              nil,
			expectedPodCount: 1,
		},
	}
	coreClient := executor.k8sClientset.client.CoreV1()
	buildIdWithPrefix := testPrefix + "-" + "1234"
	for _, test := range tests {
		got, err := executor.Start(test.request)
		assert.IsType(t, test.expectedNode, got)
		assert.IsType(t, test.err, err)
		list, _ := coreClient.Nodes().List(context.TODO(), metav1.ListOptions{})
		for _, node := range list.Items {
			assert.Equal(t, test.expectedNode, node.Name)
		}
		//test.request["namespace"].(string)
		pods, err := coreClient.Pods(testNamespace).List(context.TODO(),
			metav1.ListOptions{LabelSelector: fmt.Sprintf("sdbuild=%v", buildIdWithPrefix)})
		assert.Equal(t, test.expectedPodCount, len(pods.Items))
	}
}

func TestStop(t *testing.T) {
	buildIdWithPrefix := testPrefix + "-" + "1234"
	podName := buildIdWithPrefix + "-erbf3"
	kubeclient := fake.NewSimpleClientset(&core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: testNamespace,
			Labels:    map[string]string{"app": "screwdriver", "tier": "builds", "sdbuild": buildIdWithPrefix},
		},
	})
	errorConfig := make(map[string]interface{})
	for key, value := range testConfig {
		errorConfig[key] = value
	}
	errorConfig["namespace"] = "default"

	executor := &awsExecutorEKS{
		k8sClientset: &k8sClientset{
			client: kubeclient,
		},
	}
	tests := []struct {
		request          map[string]interface{}
		expectedPodCount int
		err              error
	}{
		{
			request:          errorConfig,
			expectedPodCount: 1,
			err:              nil,
		},
		{
			request:          testConfig,
			expectedPodCount: 0,
			err:              nil,
		},
	}

	coreClient := executor.k8sClientset.client.CoreV1()
	for _, test := range tests {
		err := executor.Stop(test.request)
		assert.IsType(t, test.err, err)
		pods, err := coreClient.Pods(testNamespace).List(context.TODO(), metav1.ListOptions{})
		assert.Equal(t, test.expectedPodCount, len(pods.Items))
	}
}
