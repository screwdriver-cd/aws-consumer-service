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
	testHTTPTimeout  = 10
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

func makeRetryableHTTPClient(maxRetries, retryWaitMin, retryWaitMax, httpTimeout int) *retryablehttp.Client {
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
	var mockStatsObj map[string]interface{}
	mockStats := []byte("{\"hostname\":\"node123\",\"imagePullStartTime\":\"2012:2332532\"}")
	_ = json.Unmarshal(mockStats, &mockStatsObj)
	var emptyStatsObj map[string]interface{}
	emptyStats := []byte("{}")
	_ = json.Unmarshal(emptyStats, &emptyStatsObj)
	var errorStatsObj map[string]interface{}
	errorStats := []byte("{\"hostname\":\"node123\",\"imagePullStartTime\":2332532}")
	_ = json.Unmarshal(errorStats, &errorStatsObj)
	tests := []struct {
		stats         map[string]interface{}
		statusMessage string
		statusCode    int
		err           error
	}{
		{mockStatsObj, "", 200, nil},
		{emptyStatsObj, "", 200, errors.New("hostname value is empty or invalid: <nil>")},
		{errorStatsObj, "", 400, errors.New("Posting to Build Stats: WARNING: received response 400 from http://fakeurl/v4/builds/15 ")},
	}

	for _, test := range tests {
		var client *retryablehttp.Client
		client = makeRetryableHTTPClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHTTPTimeout)
		client.HTTPClient = makeFakeHTTPClient(t, test.statusCode, "{}")
		testAPI := SDAPI{"http://fakeurl", "faketoken", client}
		err := testAPI.UpdateBuild(test.stats, 15, test.statusMessage)

		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("Unexpected error from UpdateBuild: \n%v\n want \n%v", err, test.err)
		}
	}
}

func TestGetAPIURL(t *testing.T) {
	var client *retryablehttp.Client
	client = makeRetryableHTTPClient(testMaxRetries, testRetryWaitMin, testRetryWaitMax, testHTTPTimeout)
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
