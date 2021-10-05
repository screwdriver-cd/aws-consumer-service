package screwdriver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/stretchr/testify/assert"
)

const (
	testMaxRetries   = 4
	testRetryWaitMin = 10
	testRetryWaitMax = 10
	testHttpTimeout  = 10
)

func makeFakeHTTPClient(t *testing.T, code int, body string) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		validateHeader(t, "Authorization", wantTokenHeader)
		w.WriteHeader(code)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, body)
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func makeValidatedFakeHTTPClient(t *testing.T, code int, body string, v func(r *http.Request)) *http.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantToken := "faketoken"
		wantTokenHeader := fmt.Sprintf("Bearer %s", wantToken)

		fmt.Println("in api")
		validateHeader(t, "Authorization", wantTokenHeader)
		v(r)

		w.WriteHeader(code)
		if code == 500 {
			time.Sleep(time.Duration(2) * time.Second)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	return &http.Client{Transport: transport}
}

func makeRetryableHttpClient(maxRetries, retryWaitMin, retryWaitMax, httpTimeout int) *retryablehttp.Client {
	client := retryablehttp.NewClient()
	client.RetryMax = maxRetries
	client.RetryWaitMin = time.Duration(retryWaitMin) * time.Millisecond
	client.RetryWaitMax = time.Duration(retryWaitMax) * time.Millisecond
	client.Backoff = retryablehttp.LinearJitterBackoff
	client.HTTPClient.Timeout = time.Duration(httpTimeout) * time.Millisecond

	return client
}

func validateHeader(t *testing.T, key, value string) func(r *http.Request) {
	return func(r *http.Request) {
		headers, ok := r.Header[key]
		if !ok {
			t.Fatalf("No %s header sent in Screwdriver request", key)
		}
		header := headers[0]
		if header != value {
			t.Errorf("%s header = %q, want %q", key, header, value)
		}
	}
}

func TestUpdateBuild(t *testing.T) {
	var stats map[string]interface{}
	mockStats := []byte("{\"Hostname\":\"node123\",\"ImagePullStartTime\":\"2012:2332532\"}")
	_ = json.Unmarshal(mockStats, &stats)

	// var stats map[string]interface{}
	// stats["Hostname"] = "node123"
	// stats["ImagePullStartTime"] = time.Now().In(UTCLoc),
	tests := []struct {
		stats         map[string]interface{}
		statusMessage string
		err           error
	}{
		{stats, "", nil},
		{stats, "Error: Build failed to start. Please check if your image is valid with curl, openssh installed and default user root or sudo NOPASSWD enabled.", nil},
		{stats, "", errors.New("Invalid build update: NOTASTATUS")},
		{stats, "", errors.New("Posting to Build Update: " +
			"WARNING: received error from PUT(http://fakeurl/v4/builds/15): " +
			"Put \"http://fakeurl/v4/builds/15\": " +
			"PUT http://fakeurl/v4/builds/15 giving up after 5 attempts ")},
	}

	for _, test := range tests {
		var client *retryablehttp.Client
		client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
		client.HTTPClient = makeFakeHTTPClient(t, 200, "{}")
		testAPI := SDAPI{"http://fakeurl", "faketoken", client}

		err := testAPI.UpdateBuild(test.stats, 15, test.statusMessage)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from UpdateBuild: \n%v\n want \n%v", err, test.err)
		}
	}
}

func TestGetAPIURL(t *testing.T) {
	var client *retryablehttp.Client
	client = makeRetryableHttpClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHttpTimeout)
	client.HTTPClient = makeValidatedFakeHTTPClient(t, 200, "{}", func(r *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r.Body)
		want := regexp.MustCompile(`{"endTime":"[\d-]+T[\d:.Z-]+","code":10}`)
		if !want.MatchString(buf.String()) {
			t.Errorf("buf.String() = %q", buf.String())
		}
	})
	testAPI := SDAPI{"http://fakeurl", "faketoken", client}
	url, _ := testAPI.GetAPIURL()

	if !reflect.DeepEqual(url, "http://fakeurl/v4/") {
		t.Errorf(`api.GetAPIURL() expected to return: "%v", instead returned "%v"`, "http://fakeurl/v4/", url)
	}
}

func TestNewDefaults(t *testing.T) {
	maxRetries = 5
	httpTimeout = time.Duration(20) * time.Second

	os.Setenv("SDAPI_TIMEOUT_SECS", "")
	os.Setenv("SDAPI_MAXRETRIES", "")
	_, _ = New("http://fakeurl", "fake")
	assert.Equal(t, httpTimeout, time.Duration(20)*time.Second)
	assert.Equal(t, maxRetries, 5)
}

func TestNew(t *testing.T) {
	os.Setenv("SDAPI_TIMEOUT_SECS", "10")
	os.Setenv("SDAPI_MAXRETRIES", "1")
	_, _ = New("http://fakeurl", "fake")
	assert.Equal(t, httpTimeout, time.Duration(10)*time.Second)
	assert.Equal(t, maxRetries, 1)
}
