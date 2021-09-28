package screwdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

const retryWaitMin = 100
const retryWaitMax = 300

var maxRetries = 5
var httpTimeout = time.Duration(20) * time.Second

type API interface {
	UpdateBuild(stats map[string]interface{}, buildID int, statusMessage string) error
}

// BuildStatusMessagePayload is a Screwdriver Build Status Message payload.
type BuildStatsPayload struct {
	Stats         map[string]interface{} `json:"stats"`
	StatusMessage string                 `json:"statusMessage,omitempty"`
}

// Token is a Screwdriver API token.
type Token struct {
	Token string `json:"token"`
}
type SDError struct {
	StatusCode int    `json:"statusCode"`
	Reason     string `json:"error"`
	Message    string `json:"message"`
}

type SDAPI struct {
	baseURL string
	token   string
	client  *retryablehttp.Client
}

func (e SDError) Error() string {
	return fmt.Sprintf("%d %s: %s", e.StatusCode, e.Reason, e.Message)
}
func tokenHeader(token string) string {
	return fmt.Sprintf("Bearer %s", token)
}

// New returns a new API object
func New(url, token string) (API, error) {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = maxRetries
	retryClient.RetryWaitMin = time.Duration(retryWaitMin) * time.Millisecond
	retryClient.RetryWaitMax = time.Duration(retryWaitMax) * time.Millisecond
	retryClient.Backoff = retryablehttp.LinearJitterBackoff
	retryClient.HTTPClient.Timeout = httpTimeout
	newapi := SDAPI{
		url,
		token,
		retryClient,
	}
	return API(newapi), nil
}
func (a SDAPI) write(url *url.URL, requestType string, bodyType string, payload io.Reader) ([]byte, error) {
	req := &http.Request{}
	buf := new(bytes.Buffer)

	size, err := buf.ReadFrom(payload)
	if err != nil {
		log.Printf("WARNING: error:[%v], not able to read payload: %v", err, payload)
		return nil, fmt.Errorf("WARNING: error:[%v], not able to read payload: %v", err, payload)
	}
	p := buf.String()

	req, err = http.NewRequest(requestType, url.String(), strings.NewReader(p))
	if err != nil {
		log.Printf("WARNING: received error generating new request for %s(%s): %v ", requestType, url.String(), err)
		return nil, fmt.Errorf("WARNING: received error generating new request for %s(%s): %v ", requestType, url.String(), err)
	}

	defer a.client.HTTPClient.CloseIdleConnections()

	req.Header.Set("Authorization", tokenHeader(a.token))
	req.Header.Set("Content-Type", bodyType)
	req.ContentLength = size

	res, err := a.client.StandardClient().Do(req)
	if res != nil {
		defer res.Body.Close()
	}

	if err != nil {
		log.Printf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
		return nil, fmt.Errorf("WARNING: received error from %s(%s): %v ", requestType, url.String(), err)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("reading response Body from Screwdriver: %v", err)
		return nil, fmt.Errorf("reading response Body from Screwdriver: %v", err)
	}

	if res.StatusCode/100 != 2 {
		var errParse SDError
		parseError := json.Unmarshal(body, &errParse)
		if parseError != nil {
			log.Printf("unparseable error response from Screwdriver: %v", parseError)
			return nil, fmt.Errorf("unparseable error response from Screwdriver: %v", parseError)
		}

		log.Printf("WARNING: received response %d from %s ", res.StatusCode, url.String())
		return nil, fmt.Errorf("WARNING: received response %d from %s ", res.StatusCode, url.String())
	}

	return body, nil
}

func (a SDAPI) makeURL(path string) (*url.URL, error) {
	version := "v4"
	fullpath := fmt.Sprintf("%s/%s/%s", a.baseURL, version, path)
	return url.Parse(fullpath)
}

func (a SDAPI) put(url *url.URL, bodyType string, payload io.Reader) ([]byte, error) {
	return a.write(url, "PUT", bodyType, payload)
}

func (a SDAPI) UpdateBuild(stats map[string]interface{}, buildID int, statusMessage string) error {
	u, err := a.makeURL(fmt.Sprintf("builds/%d", buildID))
	if err != nil {
		return fmt.Errorf("creating url: %v", err)
	}

	var payload []byte
	bs := &BuildStatsPayload{}
	if stats["Hostname"].(string) != "" {
		bs.Stats = stats
	}

	if statusMessage != "" {
		bs.StatusMessage = statusMessage
	}
	payload, err = json.Marshal(bs)
	if err != nil {
		return fmt.Errorf("Marshaling JSON for Build Stats: %v", err)
	}

	log.Printf("bs: %+v", bs)
	log.Printf("payload: %v", string(payload))

	_, err = a.put(u, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("Posting to Build Stats: %v", err)
	}

	return nil
}
