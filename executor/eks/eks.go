package executorawseks

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	core "k8s.io/api/core/v1"
	resource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
)

type ExecutorAwsEKS interface {
	Start(config map[string]interface{}) error
	Stop(config map[string]interface{}) error
}
type awsEKS struct {
	name         string
	svcEks       *eks.EKS
	k8sClientSet *kubernetes.Clientset
}

func (e *awsEKS) newClientSet(config map[string]interface{}) (*kubernetes.Clientset, error) {
	if e.k8sClientSet != nil {
		return e.k8sClientSet, nil
	}
	//connect to cluster
	cluster_info, err := e.svcEks.DescribeCluster(&eks.DescribeClusterInput{
		Name: aws.String(config["ClusterName"].(string)),
	})
	certificate := cluster_info.Cluster.CertificateAuthority.Data
	endpoint := cluster_info.Cluster.Endpoint
	arn := cluster_info.Cluster.Arn
	if err != nil {
		return nil, fmt.Errorf("Error calling DescribeCluster:: %v", err)
	}
	log.Printf("Cluster Arn: %v", string(*arn))
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, err
	}
	opts := &token.GetTokenOptions{
		ClusterID: aws.StringValue(cluster_info.Cluster.Name),
	}
	tok, err := gen.GetWithOptions(opts)
	if err != nil {
		return nil, err
	}
	log.Printf("Token: %v", tok.Token)
	ca, err := base64.StdEncoding.DecodeString(aws.StringValue(certificate))
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(
		&rest.Config{
			Host:        aws.StringValue(endpoint),
			BearerToken: tok.Token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: ca,
			},
		},
	)
	if err != nil {
		log.Fatalf("Error creating clientset: %v", err)
	}
	e.k8sClientSet = clientset

	return e.k8sClientSet, nil
}

func getPodObject(config map[string]interface{}, namespace string) *core.Pod {
	buildIdWithPrefix := config["Prefix"].(string) + "-" + config["BuildId"].(string)
	podName := buildIdWithPrefix + "-" + rand.String(5)
	return &core.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    map[string]string{"app": "screwdriver", "tier": "builds", "sdbuild": buildIdWithPrefix},
		},
		Spec: core.PodSpec{
			ServiceAccountName:            config["ServiceAccountName"].(string),
			AutomountServiceAccountToken:  &[]bool{true}[0],
			TerminationGracePeriodSeconds: &[]int64{core.DefaultTerminationGracePeriodSeconds}[0],
			RestartPolicy:                 core.RestartPolicyNever,
			DNSPolicy:                     core.DNSClusterFirst,
			Containers: []core.Container{
				core.Container{
					Name:            buildIdWithPrefix,
					Image:           config["Container"].(string),
					ImagePullPolicy: core.PullAlways,
					Ports: []core.ContainerPort{
						{
							Name:          "http",
							Protocol:      core.ProtocolTCP,
							ContainerPort: 80,
						},
					},
					SecurityContext: &core.SecurityContext{
						Privileged: config["PrivilegedMode"].(*bool),
					},
					Resources: core.ResourceRequirements{
						Limits: map[core.ResourceName]resource.Quantity{
							core.ResourceCPU:    resource.MustParse(config["CpuLimit"].(string)),
							core.ResourceMemory: resource.MustParse(config["MemoryLimit"].(string)),
						},
					},
					Env: []core.EnvVar{
						core.EnvVar{Name: "SD_RUNTIME_CLASS", Value: ""},
						core.EnvVar{Name: "SD_PUSHGATEWAY_URL", Value: ""},
						core.EnvVar{Name: "SD_TERMINATION_GRACE_PERIOD_SECONDS", Value: "60"}, //string(core.DefaultTerminationGracePeriodSeconds)
						core.EnvVar{Name: "CONTAINER_IMAGE", Value: config["Container"].(string)},
						core.EnvVar{Name: "SD_PIPELINE_ID", Value: config["PipelineId"].(string)},
						core.EnvVar{Name: "SD_BUILD_PREFIX", Value: config["Prefix"].(string)},
						core.EnvVar{Name: "NODE_ID", ValueFrom: &core.EnvVarSource{FieldRef: &core.ObjectFieldSelector{FieldPath: "spec.nodeName"}}},
						core.EnvVar{Name: "SD_BASE_COMMAND_PATH", Value: "/sd/commands/"},
						core.EnvVar{Name: "SD_TEMP", Value: "/opt/sd_tmp"},
						core.EnvVar{Name: "DOCKER_HOST", Value: "tcp"},
					},
					Command: []string{"/opt/sd/launcher_entrypoint.sh"},
					Args: []string{
						fmt.Sprintf("/opt/sd/run.sh %v %v %v %v %v %v",
							config["Token"].(string),
							config["ApiUri"].(string),
							config["StoreUri"].(string),
							config["BuildTimeout"].(string),
							config["BuildId"].(string),
							config["UiUri"].(string),
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
					Image:   config["LauncherImage"].(string),
					Command: []string{"/bin/sh", "-c", "echo launcher_start_ts:`date +%s` > /workspace/metrics && if ! [ -f /opt/launcher/launch ]; then TEMP_DIR=`mktemp -d -p /opt/launcher` && cp -a /opt/sd/* $TEMP_DIR && mkdir -p $TEMP_DIR/hab && cp -a /hab/* $TEMP_DIR/hab && mv $TEMP_DIR/* /opt/launcher && rm -rf $TEMP_DIR || true; else ls /opt/launcher; fi; echo launcher_end_ts:`date +%s` >> /workspace/metrics"},
					VolumeMounts: []core.VolumeMount{
						core.VolumeMount{MountPath: "/opt/launcher", Name: "screwdriver"},
						core.VolumeMount{MountPath: "/workspace", Name: "workspace"},
					},
				},
			},
			Volumes: []core.Volume{
				core.Volume{Name: "screwdriver", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/opt/screwdriver/sdlauncher/" + config["LauncherVersion"].(string)}}}, //Type: &core.HostPathType("DirectoryOrCreate")
				core.Volume{Name: "sdtemp", VolumeSource: core.VolumeSource{HostPath: &core.HostPathVolumeSource{Path: "/opt/screwdriver/tmp_" + config["BuildId"].(string)}}},
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
func (e *awsEKS) Start(config map[string]interface{}) (string, error) {
	clientset, _ := e.newClientSet(config)
	namespace := "sd-builds"
	// nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	// if err != nil {
	// 	log.Printf("Error getting EKS nodes: %v", err)
	// }
	// log.Printf("There are %d nodes associated with cluster", len(nodes.Items))

	podsClient := clientset.CoreV1().Pods(namespace)
	log.Printf("Namespace: %v, PodClient: +%v", namespace, &podsClient)

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("Error listing pods %v", err)
	}
	log.Printf("pods %+v\n", pods)

	log.Printf("There are %d pods in the cluster\n", len(pods.Items))

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

	log.Printf("Get pod response %v.\n", getResponse)

	nodeName := getResponse.Spec.NodeName

	log.Printf("Node %v.\n", nodeName)
	return nodeName, nil
}

// Stops a k8s pod in eks
func (e *awsEKS) Stop(config map[string]interface{}) error {
	clientset, _ := e.newClientSet(config)
	namespace := "sd-builds"
	buildIdWithPrefix := config["Prefix"].(string) + "-" + config["BuildId"].(string)
	podsClient := clientset.CoreV1().Pods(namespace)
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
func (e *awsEKS) Name() string {
	return e.name
}

func New() *awsEKS {
	sess, _ := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION"))},
	)
	svcEks := eks.New(sess)
	return &awsEKS{
		svcEks: svcEks,
		name:   "eks",
	}
}
