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
	"strings"
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

// List Executors
var executorsList = func(region string) []IExecutor {
	return []IExecutor{eksExecutor.New(region), slsExecutor.New(region)}
}

// GetExecutor selects the executor based on the executor name
func GetExecutor(name string, region string) IExecutor {
	var currentExecutor IExecutor
	executors := executorsList(region)
	for _, v := range executors {
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
	defer recoverPanic()
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
			"container":          "aws/codebuild/standard:5.0",
			"serviceAccountName": "default",
		},
	}

	decoder := json.NewDecoder(strings.NewReader(message))
	decoder.UseNumber()
	if err := decoder.Decode(&buildMesage); err != nil {
		log.Fatal(err)
	}
	buildConfig := buildMesage.BuildConfig
	provider := buildConfig["provider"].(map[string]interface{})

	messageProviderDefaults := `{
		"executorLogs":             false,
		"dlc":                      false,
		"privilegedMode":           false,
		"prune":                    true,
		"imagePullCredentialsType": "SERVICE_ROLE",
		"environmentType":          "LINUX_CONTAINER",
		"computeType":              "BUILD_GENERAL1_SMALL",
		"queuedTimeout":            5,
		"launcherComputeType":      "BUILD_GENERAL1_SMALL",
		"buildRegion" : 			"",
		"debugSession" : 			false
	}`

	var providerDefaults map[string]interface{}
	pDecoder := json.NewDecoder(strings.NewReader(messageProviderDefaults))
	pDecoder.UseNumber()
	if err := pDecoder.Decode(&providerDefaults); err != nil {
		log.Fatal(err)
	}

	for k, v := range providerDefaults {
		if provider[k] == nil {
			provider[k] = v
		}
	}
	buildConfig["provider"] = provider

	job := buildMesage.Job
	executorType := buildMesage.ExecutorType

	log.Printf("Job Type: %v, Executor: %v, Build Config: %#v", job, executorType, buildConfig)

	buildRegion := provider["buildRegion"].(string)
	if buildRegion == "" {
		buildRegion = provider["region"].(string)
	}

	if executorType != "" && job != "" {
		var hostname string
		executor := GetExecutor(executorType, buildRegion)
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
		buildID, _ := buildConfig["buildId"].(json.Number).Int64()
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
}

// HandleRequest is a go lambda request handler with event type map[string][]KafkaRecord
func HandleRequest(ctx context.Context, request events.KafkaEvent) (string, error) {
	eventSize := unsafe.Sizeof(request)
	log.Printf("size of event received %v", eventSize)

	defer finalRecover()

	var totalRecords int
	for k, record := range request.Records {
		var wg sync.WaitGroup
		count := len(record)
		totalRecords += count
		log.Printf("Received %v records for key %v", count, k)
		wg.Add(count)
		for i := 0; i < count; i++ {
			log.Printf("Record: %v", record[i])
			go ProcessMessage(i, record[i].Value, &wg, ctx)
		}
		wg.Wait()

	}
	log.Printf("Finished processing %v records", totalRecords)

	return fmt.Sprintf("Finished processing messages: %v", totalRecords), nil
}

// main function for go lambda
func main() {
	lambda.Start(HandleRequest)
}
