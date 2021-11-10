package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
	"unsafe"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	eksExecutor "github.com/screwdriver-cd/aws-consumer-service/executor/eks"
	slsExecutor "github.com/screwdriver-cd/aws-consumer-service/executor/serverless"
	sd "github.com/screwdriver-cd/aws-consumer-service/screwdriver"
)

var utcLoc, _ = time.LoadLocation("UTC")
var api = sd.New
var executorsList = []IExecutor{eksExecutor.New(), slsExecutor.New()}

// BuildMessage structure definition
type BuildMessage struct {
	Job          string                 `json:"job"`
	BuildConfig  map[string]interface{} `json:"buildConfig"`
	ExecutorType string                 `json:"executorType"`
}

// IExecutor interface with method definition
type IExecutor interface {
	Start(config map[string]interface{}) (string, error)
	Stop(config map[string]interface{}) error
	Name() string
}

// GetExecutor selects the executor based on the executor name
func GetExecutor(name string) IExecutor {
	var currentExecutor IExecutor
	for _, v := range executorsList {
		if v.Name() == name {
			currentExecutor = v
		}
	}

	return currentExecutor
}

// UpdateBuildStats calls SD API to update stats
func UpdateBuildStats(hostname string, buildID int, api sd.API) {
	if hostname != "" { // update SD stats
		stats := map[string]interface{}{
			"hostname":           hostname,
			"imagePullStartTime": time.Now().In(utcLoc),
		}
		if apierr := api.UpdateBuild(stats, int(buildID), ""); apierr != nil {
			log.Printf("Updating build stats: %v", apierr)
		}
	}
}

// ProcessMessage receives messages from the kafka broker endpoint and processes them
var ProcessMessage = func(id int, value string, wg *sync.WaitGroup, ctx context.Context) error {
	defer wg.Done()

	log.Printf("Processing id: %v", id)
	str, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		log.Printf("key: %v Base64 Decode Error:%v", id, err)
	}

	message := string(str)

	log.Printf("executor processing:%v", message)

	buildMesage := &BuildMessage{
		BuildConfig: map[string]interface{}{
			//default values
			"container":                "aws/codebuild/standard:5.0",
			"isPR":                     false,
			"privilegedMode":           false,
			"imagePullCredentialsType": "SERVICE_ROLE",
			"environmentType":          "LINUX_CONTAINER",
			"computeType":              "BUILD_GENERAL1_SMALL",
			"queuedTimeout":            "5",
			"launcherComputeType":      "BUILD_GENERAL1_SMALL",
			"logsEnabled":              false,
			"prune":                    false,
			"serviceAccountName":       "default",
		},
	}
	json.Unmarshal([]byte(message), buildMesage)
	buildConfig := buildMesage.BuildConfig
	job := buildMesage.Job
	executorType := buildMesage.ExecutorType
	log.Printf("Job Type: %v, Executor: %v, Build Config: %+v", job, executorType, buildConfig)

	if executorType != "" && job != "" {
		var hostname string
		executor := GetExecutor(executorType)
		switch string(job) {
		case "start":
			hostname, err = executor.Start(buildConfig)
		case "stop":
			err = executor.Stop(buildConfig)
		}
		if err != nil {
			log.Printf("Failed to %v build %v", job, err)
		} else {
			log.Printf("%v build successful", job)
		}
		buildID := buildConfig["buildId"].(float64)
		api, _ := api(buildConfig["apiUri"].(string), buildConfig["token"].(string))
		UpdateBuildStats(hostname, int(buildID), api)
	}

	return nil
}

//recovers panic
func recoverPanic() {
	if p := recover(); p != nil {
		filename := fmt.Sprintf("stacktrace-%s", time.Now().Format(time.RFC3339))
		tracefile := filepath.Join(os.TempDir(), filename)

		log.Printf("ERROR: Internal Screwdriver error. Please file a bug about this: %v", p)
		log.Printf("ERROR: Writing StackTrace to %s", tracefile)
		err := ioutil.WriteFile(tracefile, debug.Stack(), 0600)
		if err != nil {
			log.Printf("ERROR: Unable to write stacktrace to file: %v", err)
		}
	}
}

// finalRecover makes one last attempt to recover from a panic.
// This should only happen if the previous recovery caused a panic.
func finalRecover() {
	if p := recover(); p != nil {
		fmt.Fprintln(os.Stderr, "ERROR: Something terrible has happened. Please file a ticket with this info:")
		fmt.Fprintf(os.Stderr, "ERROR: %v\n%v\n", p, debug.Stack())
	}
	// os.Exit(0)
}

// HandleRequest is a go lambda request handler with event type map[string][]KafkaRecord
func HandleRequest(ctx context.Context, request events.KafkaEvent) (string, error) {
	eventSize := unsafe.Sizeof(request)
	log.Printf("size of event received %v", eventSize)

	defer finalRecover()
	defer recoverPanic()

	var count int
	for k, record := range request.Records {
		var wg sync.WaitGroup
		count = len(record)
		log.Printf("Received %v records for key %v", count, k)
		wg.Add(count)
		for i := 0; i < count; i++ {
			log.Printf("Record: %v", record[i])
			go ProcessMessage(i, record[i].Value, &wg, ctx)
		}
		wg.Wait()

	}
	log.Printf("Finished processing %v records", count)

	return fmt.Sprintf("Finished processing messages: %v", count), nil
}

// main function for go lambda
func main() {
	lambda.Start(HandleRequest)
}
