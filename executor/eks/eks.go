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

type eksClient struct {
	service eksiface.EKSAPI
}
type k8sClientset struct {
	client kubernetes.Interface
}
type awsExecutorEKS struct {
	name         string
	eksClient    *eksClient
	k8sClientset *k8sClientset
}

func (c *eksClient) describeCluster(cluster_name string) (*eks.DescribeClusterOutput, error) {
	if cluster_name == "" {
		return nil, errors.New("cluster name is empty")
	}
	cluster_info, err := c.service.DescribeCluster(&eks.DescribeClusterInput{
		Name: aws.String(cluster_name),
	})
	if err != nil {
		log.Printf("DescribeCluster error - %s", err)
		return nil, err
	}
	if cluster_info == nil {
		log.Printf("cluster does not exist")
		return nil, errors.New("cluster does not exist")
	}

	return cluster_info, err
}

// newAWSService returns a new instance of emr
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
func (e *awsExecutorEKS) getToken(cluster_name *string) (string, error) {
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return "", err
	}
	opts := &token.GetTokenOptions{
		ClusterID: aws.StringValue(cluster_name),
	}
	tok, err := gen.GetWithOptions(opts)
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}
func (e *awsExecutorEKS) newClientSet(config map[string]interface{}) (*k8sClientset, error) {
	if e.k8sClientset != nil {
		return e.k8sClientset, nil
	}
	//connect to cluster
	cluster_info, err := e.eksClient.describeCluster(config["clusterName"].(string))
	if err != nil {
		return nil, fmt.Errorf("Error calling DescribeCluster:%v", err)
	}
	certificate := cluster_info.Cluster.CertificateAuthority.Data
	endpoint := cluster_info.Cluster.Endpoint
	arn := cluster_info.Cluster.Arn
	log.Printf("Cluster Arn: %v", string(*arn))

	//get token
	token, err := e.getToken(cluster_info.Cluster.Name)
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

func getPodObject(config map[string]interface{}, namespace string) *core.Pod {
	buildIdStr := strconv.Itoa(config["buildId"].(int))
	buildIdWithPrefix := config["prefix"].(string) + "-" + buildIdStr
	podName := buildIdWithPrefix + "-" + rand.String(5)

	return &core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    map[string]string{"app": "screwdriver", "tier": "builds", "sdbuild": buildIdWithPrefix},
		},
		Spec: core.PodSpec{
			ServiceAccountName:            config["serviceAccountName"].(string),
			AutomountServiceAccountToken:  &[]bool{true}[0],
			TerminationGracePeriodSeconds: &[]int64{core.DefaultTerminationGracePeriodSeconds}[0],
			RestartPolicy:                 core.RestartPolicyNever,
			DNSPolicy:                     core.DNSClusterFirst,
			Containers: []core.Container{
				core.Container{
					Name:            buildIdWithPrefix,
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
						core.EnvVar{Name: "SD_RUNTIME_CLASS", Value: ""},
						core.EnvVar{Name: "SD_PUSHGATEWAY_URL", Value: ""},
						core.EnvVar{Name: "SD_TERMINATION_GRACE_PERIOD_SECONDS", Value: "60"}, //string(core.DefaultTerminationGracePeriodSeconds)
						core.EnvVar{Name: "CONTAINER_IMAGE", Value: config["container"].(string)},
						core.EnvVar{Name: "SD_PIPELINE_ID", Value: config["pipelineId"].(string)},
						core.EnvVar{Name: "SD_BUILD_PREFIX", Value: config["prefix"].(string)},
						core.EnvVar{Name: "NODE_ID", ValueFrom: &core.EnvVarSource{FieldRef: &core.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
						core.EnvVar{Name: "SD_BASE_COMMAND_PATH", Value: "/sd/commands/"},
						core.EnvVar{Name: "SD_TEMP", Value: "/opt/sd_tmp"},
						core.EnvVar{Name: "DOCKER_HOST", Value: "tcp"},
					},
					Command: []string{"/opt/sd/launcher_entrypoint.sh"},
					Args: []string{
						fmt.Sprintf("/opt/sd/run.sh %v %v %v %v %v %v",
							config["token"].(string),
							config["apiUri"].(string),
							config["storeUri"].(string),
							config["buildTimeout"].(string),
							buildIdStr,
							config["uiUri"].(string),
						),
					},
					VolumeMounts: []core.VolumeMount{
						core.VolumeMount{Name: "podinfo", MountPath: "/etc/podinfo", ReadOnly: true},
						core.VolumeMount{MountPath: "/opt/sd", Name: "screwdriver", ReadOnly: true},
						core.VolumeMount{MountPath: "/opt/sd_tmp", Name: "sdtemp"},
						core.VolumeMount{MountPath: "/workspace", Name: "workspace"},
					},
				},
			},
			InitContainers: []core.Container{
				core.Container{
					Name:    "launcher-" + buildIdWithPrefix,
					Image:   config["launcherImage"].(string),
					Command: []string{"/bin/sh", "-c", "echo launcher_start_ts:`date +%s` > /workspace/metrics && if ! [ -f /opt/launcher/launch ]; then TEMP_DIR=`mktemp -d -p /opt/launcher` && cp -a /opt/sd/* $TEMP_DIR && mkdir -p $TEMP_DIR/hab && cp -a /hab/* $TEMP_DIR/hab && mv $TEMP_DIR/* /opt/launcher && rm -rf $TEMP_DIR || true; else ls /opt/launcher; fi; echo launcher_end_ts:`date +%s` >> /workspace/metrics"},
					VolumeMounts: []core.VolumeMount{
						core.VolumeMount{MountPath: "/opt/launcher", Name: "screwdriver"},
						core.VolumeMount{MountPath: "/workspace", Name: "workspace"},
					},
				},
			},
			Volumes: []core.Volume{
				core.Volume{Name: "screwdriver", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/opt/screwdriver/sdlauncher/" + config["launcherVersion"].(string)}}}, //Type: &core.HostPathType("DirectoryOrCreate")
				core.Volume{Name: "sdtemp", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/opt/screwdriver/tmp_" + buildIdStr}}},
				core.Volume{Name: "workspace", VolumeSource: core.VolumeSource{EmptyDir: &core.EmptyDirVolumeSource{}}},
				core.Volume{Name: "podinfo", VolumeSource: core.VolumeSource{DownwardAPI: &core.DownwardAPIVolumeSource{Items: []core.DownwardAPIVolumeFile{
					core.DownwardAPIVolumeFile{Path: "labels", FieldRef: &core.ObjectFieldSelector{FieldPath: "metadata.labels"}},
					core.DownwardAPIVolumeFile{Path: "annotations", FieldRef: &core.ObjectFieldSelector{FieldPath: "metadata.annotations"}},
				}}}},
			},
		},
	}
}

// Start a k8s pod in eks
func (e *awsExecutorEKS) Start(config map[string]interface{}) (string, error) {
	clientset, _ := e.newClientSet(config)
	namespace := config["namespace"].(string)
	podsClient := clientset.client.CoreV1().Pods(namespace)
	log.Printf("Namespace: %v, PodClient: +%v", namespace, &podsClient)

	// build the pod defination we want to deploy
	pod := getPodObject(config, namespace)
	log.Printf("Pod spec %+v", pod)
	// create pod in eks cluster
	fmt.Println("Creating pod...")
	podResponse, errPod := podsClient.Create(context.TODO(), pod, metav1.CreateOptions{})
	if errPod != nil {
		fmt.Printf("Error creating pod %v", errPod)
	}
	log.Printf("Created pod %v.\n", podResponse.ObjectMeta.Name)

	getResponse, _ := podsClient.Get(context.TODO(), podResponse.ObjectMeta.Name, metav1.GetOptions{})

	log.Printf("Get pod response %+v.\n", getResponse.Spec)

	nodeName := getResponse.Spec.NodeName

	log.Printf("Node %v.\n", nodeName)
	return nodeName, nil
}

// Stops a k8s pod in eks
func (e *awsExecutorEKS) Stop(config map[string]interface{}) error {
	clientset, _ := e.newClientSet(config)
	namespace := config["namespace"].(string)
	buildIdStr := strconv.Itoa(config["buildId"].(int))
	buildIdWithPrefix := config["prefix"].(string) + "-" + buildIdStr
	podsClient := clientset.client.CoreV1().Pods(namespace)
	listPods, err := podsClient.List(context.TODO(), metav1.ListOptions{LabelSelector: fmt.Sprintf("sdbuild=%v", buildIdWithPrefix)})
	if err != nil {
		log.Printf("List pod response %v.\n", listPods)
	}
	//delete pod in eks cluster
	fmt.Println("Deleting pod...")
	for _, i := range listPods.Items {
		result := podsClient.Delete(context.TODO(), i.Name, metav1.DeleteOptions{})
		fmt.Printf("Deleted pod %q.\n", result)
	}

	return nil
}

//Returns the name of executor
func (e *awsExecutorEKS) Name() string {
	return e.name
}

func New() *awsExecutorEKS {
	return &awsExecutorEKS{
		eksClient: newEKSService(os.Getenv("AWS_REGION")),
		name:      "eks",
	}
}
