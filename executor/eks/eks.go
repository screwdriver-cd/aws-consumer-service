package eks

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/eks/eksiface"
	core "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	//"k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
)

const (
	executorName = "eks"
)

// eks client definition struct
type eksClient struct {
	service eksiface.EKSAPI
}

// k8s clientset definition struct
type k8sClientset struct {
	client kubernetes.Interface
}

// AwsExecutorEKS definition struct
type AwsExecutorEKS struct {
	name         string
	eksClient    *eksClient
	k8sClientset *k8sClientset
}

// describes an eks cluster
func (c *eksClient) describeCluster(clusterName string) (*eks.DescribeClusterOutput, error) {
	if clusterName == "" {
		return nil, errors.New("cluster name is empty")
	}
	clusterInfo, err := c.service.DescribeCluster(&eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	})
	if err != nil {
		log.Printf("DescribeCluster error - %s", err)
		return nil, err
	}
	if clusterInfo == nil {
		log.Printf("cluster does not exist")
		return nil, errors.New("cluster does not exist")
	}

	return clusterInfo, err
}

// newAWSService returns a new instance of eks
func newEKSService(region string) *eksClient {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region)},
	)
	if err != nil {
		log.Printf("error while creating AWS session - %s", err.Error())
	}
	svcEks := eks.New(sess)

	return &eksClient{
		service: svcEks,
	}
}

// gets the token for creating clientset
func (e *AwsExecutorEKS) getToken(clusterName *string) (string, error) {
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return "", err
	}
	opts := &token.GetTokenOptions{
		ClusterID: aws.StringValue(clusterName),
	}
	tok, err := gen.GetWithOptions(opts)
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}

// Returns a new client set for kubernetes
func (e *AwsExecutorEKS) newClientSet(config map[string]interface{}) (*k8sClientset, error) {
	if e.k8sClientset != nil {
		return e.k8sClientset, nil
	}
	//connect to cluster
	clusterInfo, err := e.eksClient.describeCluster(config["clusterName"].(string))
	if err != nil {
		return nil, fmt.Errorf("Error calling DescribeCluster:%v", err)
	}
	certificate := clusterInfo.Cluster.CertificateAuthority.Data
	endpoint := clusterInfo.Cluster.Endpoint
	arn := clusterInfo.Cluster.Arn
	log.Printf("Cluster Arn: %v", string(*arn))

	//get token
	token, err := e.getToken(clusterInfo.Cluster.Name)
	ca, err := base64.StdEncoding.DecodeString(aws.StringValue(certificate))
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(
		&rest.Config{
			Host:        aws.StringValue(endpoint),
			BearerToken: token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: ca,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("Error creating clientset: %v", err)
	}
	e.k8sClientset = &k8sClientset{
		client: clientset,
	}

	return e.k8sClientset, nil
}

// gets the pod object for creating pod
func getPodObject(config map[string]interface{}, namespace string) *core.Pod {
	buildIDStr := strconv.Itoa(config["buildId"].(int))
	buildIDWithPrefix := config["prefix"].(string) + "-" + buildIDStr
	podName := buildIDWithPrefix + "-" + rand.String(5)

	return &core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    map[string]string{"app": "screwdriver", "tier": "builds", "sdbuild": buildIDWithPrefix},
		},
		Spec: core.PodSpec{
			ServiceAccountName:            config["serviceAccountName"].(string),
			AutomountServiceAccountToken:  &[]bool{true}[0],
			TerminationGracePeriodSeconds: &[]int64{core.DefaultTerminationGracePeriodSeconds}[0],
			RestartPolicy:                 core.RestartPolicyNever,
			DNSPolicy:                     core.DNSClusterFirst,
			Containers: []core.Container{
				{
					Name:            buildIDWithPrefix,
					Image:           config["container"].(string),
					ImagePullPolicy: core.PullAlways,
					Ports: []core.ContainerPort{
						{
							Name:          "http",
							Protocol:      core.ProtocolTCP,
							ContainerPort: 80,
						},
					},
					SecurityContext: &core.SecurityContext{
						Privileged: &[]bool{config["privilegedMode"].(bool)}[0],
					},
					Resources: core.ResourceRequirements{
						Limits: map[core.ResourceName]resource.Quantity{
							core.ResourceCPU:    resource.MustParse(config["cpuLimit"].(string)),
							core.ResourceMemory: resource.MustParse(config["memoryLimit"].(string)),
						},
					},
					Env: []core.EnvVar{
						{Name: "SD_RUNTIME_CLASS", Value: ""},
						{Name: "SD_PUSHGATEWAY_URL", Value: ""},
						{Name: "SD_TERMINATION_GRACE_PERIOD_SECONDS", Value: "60"}, //string(core.DefaultTerminationGracePeriodSeconds)
						{Name: "CONTAINER_IMAGE", Value: config["container"].(string)},
						{Name: "SD_PIPELINE_ID", Value: config["pipelineId"].(string)},
						{Name: "SD_BUILD_PREFIX", Value: config["prefix"].(string)},
						{Name: "NODE_ID", ValueFrom: &core.EnvVarSource{FieldRef: &core.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
						{Name: "SD_BASE_COMMAND_PATH", Value: "/sd/commands/"},
						{Name: "SD_TEMP", Value: "/opt/sd_tmp"},
						{Name: "DOCKER_HOST", Value: "tcp"},
					},
					Command: []string{"/opt/sd/launcher_entrypoint.sh"},
					Args: []string{
						fmt.Sprintf("/opt/sd/run.sh %v %v %v %v %v %v",
							config["token"].(string),
							config["apiUri"].(string),
							config["storeUri"].(string),
							config["buildTimeout"].(string),
							buildIDStr,
							config["uiUri"].(string),
						),
					},
					VolumeMounts: []core.VolumeMount{
						{Name: "podinfo", MountPath: "/etc/podinfo", ReadOnly: true},
						{MountPath: "/opt/sd", Name: "screwdriver", ReadOnly: true},
						{MountPath: "/opt/sd_tmp", Name: "sdtemp"},
						{MountPath: "/workspace", Name: "workspace"},
					},
				},
			},
			InitContainers: []core.Container{
				{
					Name:    "launcher-" + buildIDWithPrefix,
					Image:   config["launcherImage"].(string),
					Command: []string{"/bin/sh", "-c", "echo launcher_start_ts:`date +%s` > /workspace/metrics && if ! [ -f /opt/launcher/launch ]; then TEMP_DIR=`mktemp -d -p /opt/launcher` && cp -a /opt/sd/* $TEMP_DIR && mkdir -p $TEMP_DIR/hab && cp -a /hab/* $TEMP_DIR/hab && mv $TEMP_DIR/* /opt/launcher && rm -rf $TEMP_DIR || true; else ls /opt/launcher; fi; echo launcher_end_ts:`date +%s` >> /workspace/metrics"},
					VolumeMounts: []core.VolumeMount{
						{MountPath: "/opt/launcher", Name: "screwdriver"},
						{MountPath: "/workspace", Name: "workspace"},
					},
				},
			},
			Volumes: []core.Volume{
				{Name: "screwdriver", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/opt/screwdriver/sdlauncher/" + config["launcherVersion"].(string)}}}, //Type: &core.HostPathType("DirectoryOrCreate")
				{Name: "sdtemp", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/opt/screwdriver/tmp_" + buildIDStr}}},
				{Name: "workspace", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
				{Name: "podinfo", VolumeSource: core.VolumeSource{DownwardAPI: &core.DownwardAPIVolumeSource{Items: []core.DownwardAPIVolumeFile{
					{Path: "labels", FieldRef: &core.ObjectFieldSelector{FieldPath: "metadata.labels"}},
					{Path: "annotations", FieldRef: &core.ObjectFieldSelector{FieldPath: "metadata.annotations"}},
				}}}},
			},
		},
	}
}

// Start a kubernetes pod in eks cluster
func (e *AwsExecutorEKS) Start(config map[string]interface{}) (string, error) {
	clientset, _ := e.newClientSet(config)
	namespace := config["namespace"].(string)
	podsClient := clientset.client.CoreV1().Pods(namespace)
	log.Printf("Namespace: %v, PodClient: +%v", namespace, &podsClient)

	// build the pod definition we want to deploy
	pod := getPodObject(config, namespace)
	log.Printf("Pod spec %+v", pod.Spec)
	// create pod in eks cluster
	log.Println("Creating pod...")
	podResponse, errPod := podsClient.Create(context.TODO(), pod, metav1.CreateOptions{})
	if errPod != nil {
		return "", fmt.Errorf("Error creating pod %v", errPod)
	}
	log.Printf("Created pod %v.\n", podResponse.ObjectMeta.Name)

	getResponse, _ := podsClient.Get(context.TODO(), podResponse.ObjectMeta.Name, metav1.GetOptions{})

	log.Printf("Get pod response %+v.\n", getResponse.Spec)

	nodeName := getResponse.Spec.NodeName

	log.Printf("Node:%v\n", nodeName)

	return nodeName, nil
}

// Stop fn deletes a kubernetes pod in eks cluster
func (e *AwsExecutorEKS) Stop(config map[string]interface{}) error {
	clientset, _ := e.newClientSet(config)
	namespace := config["namespace"].(string)
	buildIDStr := strconv.Itoa(config["buildId"].(int))
	buildIDWithPrefix := config["prefix"].(string) + "-" + buildIDStr

	podsClient := clientset.client.CoreV1().Pods(namespace)
	listPods, err := podsClient.List(context.TODO(), metav1.ListOptions{LabelSelector: fmt.Sprintf("sdbuild=%v", buildIDWithPrefix)})

	if err != nil {
		return fmt.Errorf("failed to get pods %v", err)
	}
	//delete pod in eks cluster
	for _, i := range listPods.Items {
		log.Printf("Deleting pod...%s", i.Name)
		result := podsClient.Delete(context.TODO(), i.Name, metav1.DeleteOptions{})
		log.Printf("Deleted pod %s", result)
	}

	return nil
}

//Name fn returns the name of executor
func (e *AwsExecutorEKS) Name() string {
	return e.name
}

// New fn returns a new instance of EKS executor
func New() *AwsExecutorEKS {
	return &AwsExecutorEKS{
		eksClient: newEKSService(os.Getenv("AWS_REGION")),
		name:      executorName,
	}
}
