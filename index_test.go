package main

import (
	"context"
	"sync"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	sd "github.com/screwdriver-cd/aws-consumer-service/screwdriver"
	"github.com/stretchr/testify/assert"
)

const (
	TestTopic   = "builds-551964337302-usw2"
	TestBuildID = 1234
)

type mockEksExecutor struct {
	name string
}
type mockSlsExecutor struct {
	name string
}

func (e *mockEksExecutor) Name() string {
	return e.name
}
func (e *mockSlsExecutor) Name() string {
	return e.name
}

var startFn string
var stopFn string
var startSlsFn string
var stopSlsFn string

func (e *mockEksExecutor) Start(config map[string]interface{}) (string, error) {
	startFn = "starteks"
	return "node123", nil
}
func (e *mockEksExecutor) Stop(config map[string]interface{}) error {
	stopFn = "stopeks"
	return nil
}
func (e *mockSlsExecutor) Start(config map[string]interface{}) (string, error) {
	startSlsFn = "startsls"
	return "proj123", nil
}
func (e *mockSlsExecutor) Stop(config map[string]interface{}) error {
	stopSlsFn = "stopsls"
	return nil
}
func newSls() *mockSlsExecutor {
	return &mockSlsExecutor{
		name: "sls",
	}
}
func newEks() *mockEksExecutor {
	return &mockEksExecutor{
		name: "eks",
	}
}

func mockAPI(t *testing.T, testBuildID int) MockAPI {
	return MockAPI{
		updateBuild: func(stats map[string]interface{}, buildID int, statusMessage string) error {
			if buildID != testBuildID {
				t.Errorf("stats[\"hostname\"] == %s, want %s", stats["hostname"], "")
				// Panic to get the stacktrace
				panic(true)
			}
			if stats["hostname"] == "" {
				t.Errorf("stats[\"hostname\"] == %s, want %s", stats["hostname"], "")
				// Panic to get the stacktrace
				panic(true)
			}
			return nil
		},
		getAPIURL: func() (string, error) {
			return "https://api.screwdriver.cd/v4/", nil
		},
	}
}

type MockAPI struct {
	updateBuild func(stats map[string]interface{}, buildID int, statusMessage string) error
	getAPIURL   func() (string, error)
}

func (f MockAPI) UpdateBuild(stats map[string]interface{}, buildID int, statusMessage string) error {
	if f.updateBuild != nil {
		return f.updateBuild(stats, buildID, "")
	}
	return nil
}
func (f MockAPI) GetAPIURL() (string, error) {
	return "", nil
}

func newSdAPI(apiURI string, token string) (sd.API, error) {
	return sd.API(MockAPI{}), nil
}

func TestHandleRequest(t *testing.T) {
	executorsList = []IExecutor{newSls(), newEks()}
	api = newSdAPI
	tests := []struct {
		request events.KafkaEvent
		expect  string
		err     error
	}{
		{
			request: events.KafkaEvent{
				Records: map[string][]events.KafkaRecord{
					"builds-551964337302-0": {
						{
							Topic:         TestTopic,
							Partition:     0,
							Offset:        12722,
							TimestampType: "CREATE_TIME",
							Key:           "a2V5LTE1NQ==",
							Value:         "ewogICAgImpvYiI6ICJzdGFydCIsCiAgICAiZXhlY3V0b3JUeXBlIjogInNscyIsCiAgICAiYnVpbGRDb25maWciOiB7CiAgICAgICAgImpvYklkIjogNjgyMiwKICAgICAgICAiam9iU3RhdGUiOiAiRU5BQkxFRCIsCiAgICAgICAgImpvYkFyY2hpdmVkIjogZmFsc2UsCiAgICAgICAgImpvYk5hbWUiOiAiUFItODpkZXBsb3kiLAogICAgICAgICJhbm5vdGF0aW9ucyI6IHsKICAgICAgICAgICAgInNjcmV3ZHJpdmVyLmNkL2V4ZWN1dG9yIjogInNscyIKICAgICAgICB9LAogICAgICAgICJibG9ja2VkQnkiOiBbCiAgICAgICAgICAgIDY4MjIKICAgICAgICBdLAogICAgICAgICJwaXBlbGluZSI6IHsKICAgICAgICAgICAgImlkIjogMTg5OCwKICAgICAgICAgICAgInNjbUNvbnRleHQiOiAiZ2l0aHViOmdpdGh1Yi5jb20iCiAgICAgICAgfSwKICAgICAgICAiZnJlZXplV2luZG93cyI6IFtdLAogICAgICAgICJhcGlVcmkiOiAiaHR0cHM6Ly9hcGkuc2NyZXdkcml2ZXIuY2QiLAogICAgICAgICJidWlsZElkIjogMzUzMjM5LAogICAgICAgICJldmVudElkIjogMzQ2ODYzLAogICAgICAgICJjb250YWluZXIiOiAiMTExMTExMTExLmRrci5lY3IudXMtZWFzdC0yLmFtYXpvbmF3cy5jb20vc2NyZXdkcml2ZXItaHViOm5vZGUxMiIsCiAgICAgICAgInBpcGVsaW5lSWQiOiAxODk4LAogICAgICAgICJpc1BSIjogdHJ1ZSwKICAgICAgICAicHJQYXJlbnRKb2JJZCI6IDY3ODksCiAgICAgICAgInByb3ZpZGVyIjogewogICAgICAgICAgICAibmFtZSI6ICJhd3MiLAogICAgICAgICAgICAicmVnaW9uIjogInVzLWVhc3QtMiIsCiAgICAgICAgICAgICJhY2NvdW50SWQiOiAxMTExMTExMTEsCiAgICAgICAgICAgICJ2cGMiOiB7CiAgICAgICAgICAgICAgICAidnBjSWQiOiAidnBjLTA5MTRjYTcxYjFmMjc0MTY3IiwKICAgICAgICAgICAgICAgICJzZWN1cml0eUdyb3VwSWRzIjogWwogICAgICAgICAgICAgICAgICAgICJzZy0wNWU0ODJlNjNhMTgwMmFhNCIKICAgICAgICAgICAgICAgIF0sCiAgICAgICAgICAgICAgICAic3VibmV0SWRzIjogWwogICAgICAgICAgICAgICAgICAgICJzdWJuZXQtMGE3YmFlZDhmNjMyZDQxYzYiLAogICAgICAgICAgICAgICAgICAgICJzdWJuZXQtMGYyMGUzYjQxMTkzNmEwMTkiCiAgICAgICAgICAgICAgICBdCiAgICAgICAgICAgIH0sCiAgICAgICAgICAgICJyb2xlIjogImFybjphd3M6aWFtOjoxMTExMTExMTE6cm9sZS9jZC5zY3Jld2RyaXZlci5jb25zdW1lci5pbnRlZ3JhdGlvbi1zZGJ1aWxkIiwKICAgICAgICAgICAgImV4ZWN1dG9yIjogInNscyIsCiAgICAgICAgICAgICJsYXVuY2hlckltYWdlIjogIjExMTExMTExMS5ka3IuZWNyLnVzLWVhc3QtMi5hbWF6b25hd3MuY29tL3NjcmV3ZHJpdmVyLWh1YjpsYXVuY2hlcnY2LjAuMTQ3IiwKICAgICAgICAgICAgImxhdW5jaGVyVmVyc2lvbiI6ICJ2Ni4wLjE0NyIsCiAgICAgICAgICAgICJleGVjdXRvckxvZ3MiOiB0cnVlLAogICAgICAgICAgICAicHJpdmlsZWdlZE1vZGUiOiBmYWxzZSwKICAgICAgICAgICAgImNvbXB1dGVUeXBlIjogIkJVSUxEX0dFTkVSQUwxX1NNQUxMIiwKICAgICAgICAgICAgImVudmlyb25tZW50IjogIkxJTlVYX0NPTlRBSU5FUiIKICAgICAgICB9LAogICAgICAgICJ0b2tlbiI6ICJ0ZXN0dG9rZW4iLAogICAgICAgICJidWlsZFRpbWVvdXQiOiA5MCwKICAgICAgICAidWlVcmkiOiAiaHR0cHM6Ly9zY3Jld2RyaXZlci5jZCIsCiAgICAgICAgInN0b3JlVXJpIjogImh0dHBzOi8vc3RvcmUuc2NyZXdkcml2ZXIuY2QiCiAgICB9Cn0=",
						},
					},
				},
			},
			expect: "Finished processing messages: 1",
			err:    nil,
		},
		{
			request: events.KafkaEvent{Records: map[string][]events.KafkaRecord{}},
			expect:  "Finished processing messages: 0",
			err:     nil,
		},
	}

	for _, test := range tests {
		response, err := HandleRequest(context.TODO(), test.request)
		assert.IsType(t, test.err, err)
		assert.Equal(t, test.expect, response)
	}
}

func TestGetExecutor(t *testing.T) {
	executorsList = []IExecutor{newSls(), newEks()}
	tests := []struct {
		name   string
		expect interface{}
		err    error
	}{
		{name: "eks", expect: &mockEksExecutor{}, err: nil},
		{name: "sls", expect: &mockSlsExecutor{}, err: nil},
	}
	for _, test := range tests {
		response := GetExecutor(test.name)
		assert.IsType(t, test.expect, response)
	}
}

func TestEksStartMessage(t *testing.T) {
	executorsList = []IExecutor{newSls(), newEks()}
	api = newSdAPI

	var wg sync.WaitGroup
	wg.Add(1)
	tests := []struct {
		id    int
		value string
		wg    *sync.WaitGroup
		err   error
	}{
		{
			id:    1,
			value: "ewogICAgImpvYiI6ICJzdGFydCIsCiAgICAiZXhlY3V0b3JUeXBlIjogImVrcyIsCiAgICAiYnVpbGRDb25maWciOiB7CiAgICAgICAgImpvYklkIjogNjgyMiwKICAgICAgICAiam9iU3RhdGUiOiAiRU5BQkxFRCIsCiAgICAgICAgImpvYkFyY2hpdmVkIjogZmFsc2UsCiAgICAgICAgImpvYk5hbWUiOiAiUFItODpkZXBsb3kiLAogICAgICAgICJhbm5vdGF0aW9ucyI6IHsKICAgICAgICAgICAgInNjcmV3ZHJpdmVyLmNkL2V4ZWN1dG9yIjogImVrcyIKICAgICAgICB9LAogICAgICAgICJibG9ja2VkQnkiOiBbCiAgICAgICAgICAgIDY4MjIKICAgICAgICBdLAogICAgICAgICJwaXBlbGluZSI6IHsKICAgICAgICAgICAgImlkIjogMTg5OCwKICAgICAgICAgICAgInNjbUNvbnRleHQiOiAiZ2l0aHViOmdpdGh1Yi5jb20iCiAgICAgICAgfSwKICAgICAgICAiZnJlZXplV2luZG93cyI6IFtdLAogICAgICAgICJhcGlVcmkiOiAiaHR0cHM6Ly9hcGkuc2NyZXdkcml2ZXIuY2QiLAogICAgICAgICJidWlsZElkIjogMzUzMjM5LAogICAgICAgICJldmVudElkIjogMzQ2ODYzLAogICAgICAgICJjb250YWluZXIiOiAiMTExMTExMTExLmRrci5lY3IudXMtZWFzdC0yLmFtYXpvbmF3cy5jb20vc2NyZXdkcml2ZXItaHViOm5vZGUxMiIsCiAgICAgICAgInBpcGVsaW5lSWQiOiAxODk4LAogICAgICAgICJpc1BSIjogdHJ1ZSwKICAgICAgICAicHJQYXJlbnRKb2JJZCI6IDY3ODksCiAgICAgICAgInByb3ZpZGVyIjogewogICAgICAgICAgICAibmFtZSI6ICJhd3MiLAogICAgICAgICAgICAicmVnaW9uIjogInVzLWVhc3QtMiIsCiAgICAgICAgICAgICJhY2NvdW50SWQiOiAxMTExMTExMTEsCiAgICAgICAgICAgICJ2cGMiOiB7CiAgICAgICAgICAgICAgICAidnBjSWQiOiAidnBjLTA5MTRjYTcxYjFmMjc0MTY3IiwKICAgICAgICAgICAgICAgICJzZWN1cml0eUdyb3VwSWRzIjogWwogICAgICAgICAgICAgICAgICAgICJzZy0wNWU0ODJlNjNhMTgwMmFhNCIKICAgICAgICAgICAgICAgIF0sCiAgICAgICAgICAgICAgICAic3VibmV0SWRzIjogWwogICAgICAgICAgICAgICAgICAgICJzdWJuZXQtMGE3YmFlZDhmNjMyZDQxYzYiLAogICAgICAgICAgICAgICAgICAgICJzdWJuZXQtMGYyMGUzYjQxMTkzNmEwMTkiCiAgICAgICAgICAgICAgICBdCiAgICAgICAgICAgIH0sCiAgICAgICAgICAgICJyb2xlIjogImFybjphd3M6aWFtOjoxMTExMTExMTE6cm9sZS9jZC5zY3Jld2RyaXZlci5jb25zdW1lci5pbnRlZ3JhdGlvbi1zZGJ1aWxkIiwKICAgICAgICAgICAgImV4ZWN1dG9yIjogImVrcyIsCiAgICAgICAgICAgICJsYXVuY2hlckltYWdlIjogIjExMTExMTExMS5ka3IuZWNyLnVzLWVhc3QtMi5hbWF6b25hd3MuY29tL3NjcmV3ZHJpdmVyLWh1YjpsYXVuY2hlcnY2LjAuMTQ3IiwKICAgICAgICAgICAgImxhdW5jaGVyVmVyc2lvbiI6ICJ2Ni4wLjE0NyIsCiAgICAgICAgICAgICJleGVjdXRvckxvZ3MiOiB0cnVlLAogICAgICAgICAgICAiY2x1c3Rlck5hbWUiOiAic2QtYnVpbGQtZWtzIiwKICAgICAgICAgICAgInByZWZpeCI6ICJiZXRhIiwKICAgICAgICAgICAgImNwdUxpbWl0IjogIjIwMDBtIiwKICAgICAgICAgICAgIm1lbW9yeUxpbWl0IjogIjJHaSIKICAgICAgICB9LAogICAgICAgICJ0b2tlbiI6ICJ0ZXN0dG9rZW4iLAogICAgICAgICJidWlsZFRpbWVvdXQiOiA5MCwKICAgICAgICAidWlVcmkiOiAiaHR0cHM6Ly9zY3Jld2RyaXZlci5jZCIsCiAgICAgICAgInN0b3JlVXJpIjogImh0dHBzOi8vc3RvcmUuc2NyZXdkcml2ZXIuY2QiCiAgICB9Cn0=",
			wg:    &wg,
			err:   nil,
		},
	}
	for _, test := range tests {
		err := ProcessMessage(test.id, test.value, test.wg, context.TODO())
		assert.Equal(t, "starteks", startFn)
		assert.IsType(t, test.err, err)
	}
}
func TestEksStopMessage(t *testing.T) {
	executorsList = []IExecutor{newSls(), newEks()}
	api = newSdAPI

	var wg sync.WaitGroup
	wg.Add(1)
	tests := []struct {
		id    int
		value string
		wg    *sync.WaitGroup
		err   error
	}{
		{
			id:    1,
			value: "ewogICAgImpvYiI6ICJzdG9wIiwKICAgICJleGVjdXRvclR5cGUiOiAiZWtzIiwKICAgICJidWlsZENvbmZpZyI6IHsKICAgICAgICAiYnVpbGRJZCI6IDM1MzIzOSwKICAgICAgICAiam9iSWQiOiA2ODIyLAogICAgICAgICJqb2JTdGF0ZSI6ICJFTkFCTEVEIiwKICAgICAgICAiam9iQXJjaGl2ZWQiOiBmYWxzZSwKICAgICAgICAiam9iTmFtZSI6ICJQUi04OmRlcGxveSIsCiAgICAgICAgImFubm90YXRpb25zIjogewogICAgICAgICAgICAic2NyZXdkcml2ZXIuY2QvZXhlY3V0b3IiOiAic2xzIgogICAgICAgIH0sCiAgICAgICAgImJsb2NrZWRCeSI6IFsKICAgICAgICAgICAgNjgyMgogICAgICAgIF0sCiAgICAgICAgInBpcGVsaW5lIjogewogICAgICAgICAgICAiaWQiOiAxODk4LAogICAgICAgICAgICAic2NtQ29udGV4dCI6ICJnaXRodWI6Z2l0aHViLmNvbSIKICAgICAgICB9LAogICAgICAgICJmcmVlemVXaW5kb3dzIjogW10sCiAgICAgICAgImFwaVVyaSI6ICJodHRwczovL2FwaS5zY3Jld2RyaXZlci5jZCIsCiAgICAgICAgImV2ZW50SWQiOiAzNDY4NjMsCiAgICAgICAgImNvbnRhaW5lciI6ICIxMTExMTExMTEuZGtyLmVjci51cy1lYXN0LTIuYW1hem9uYXdzLmNvbS9zY3Jld2RyaXZlci1odWI6bm9kZTEyIiwKICAgICAgICAicGlwZWxpbmVJZCI6IDE4OTgsCiAgICAgICAgImlzUFIiOiB0cnVlLAogICAgICAgICJwclBhcmVudEpvYklkIjogNjc4OSwKICAgICAgICAicHJvdmlkZXIiOiB7CiAgICAgICAgICAgICJuYW1lIjogImF3cyIsCiAgICAgICAgICAgICJyZWdpb24iOiAidXMtZWFzdC0yIiwKICAgICAgICAgICAgImFjY291bnRJZCI6IDExMTExMTExMSwKICAgICAgICAgICAgInZwYyI6IHsKICAgICAgICAgICAgICAgICJ2cGNJZCI6ICJ2cGMtMDkxNGNhNzFiMWYyNzQxNjciLAogICAgICAgICAgICAgICAgInNlY3VyaXR5R3JvdXBJZHMiOiBbCiAgICAgICAgICAgICAgICAgICAgInNnLTA1ZTQ4MmU2M2ExODAyYWE0IgogICAgICAgICAgICAgICAgXSwKICAgICAgICAgICAgICAgICJzdWJuZXRJZHMiOiBbCiAgICAgICAgICAgICAgICAgICAgInN1Ym5ldC0wYTdiYWVkOGY2MzJkNDFjNiIsCiAgICAgICAgICAgICAgICAgICAgInN1Ym5ldC0wZjIwZTNiNDExOTM2YTAxOSIKICAgICAgICAgICAgICAgIF0KICAgICAgICAgICAgfSwKICAgICAgICAgICAgInJvbGUiOiAiYXJuOmF3czppYW06OjExMTExMTExMTpyb2xlL2NkLnNjcmV3ZHJpdmVyLmNvbnN1bWVyLmludGVncmF0aW9uLXNkYnVpbGQiLAogICAgICAgICAgICAiZXhlY3V0b3IiOiAiZWtzIiwKICAgICAgICAgICAgImxhdW5jaGVySW1hZ2UiOiAiMTExMTExMTExLmRrci5lY3IudXMtZWFzdC0yLmFtYXpvbmF3cy5jb20vc2NyZXdkcml2ZXItaHViOmxhdW5jaGVydjYuMC4xNDciLAogICAgICAgICAgICAibGF1bmNoZXJWZXJzaW9uIjogInY2LjAuMTQ3IiwKICAgICAgICAgICAgImV4ZWN1dG9yTG9ncyI6IHRydWUsCiAgICAgICAgICAgICJjbHVzdGVyTmFtZSI6ICJzZC1idWlsZC1la3MiLAogICAgICAgICAgICAiY3B1TGltaXQiOiAiMjAwMG0iLAogICAgICAgICAgICAibWVtb3J5TGltaXQiOiAiMkdpIgogICAgICAgIH0sCiAgICAgICAgInRva2VuIjogInNhbXBsZXRva2VuIiwKICAgICAgICAiYnVpbGRUaW1lb3V0IjogOTAsCiAgICAgICAgInVpVXJpIjogImh0dHBzOi8vc2NyZXdkcml2ZXIuY2QiLAogICAgICAgICJzdG9yZVVyaSI6ICJodHRwczovL3N0b3JlLnNjcmV3ZHJpdmVyLmNkIgogICAgfQp9",
			wg:    &wg,
			err:   nil,
		},
	}
	for _, test := range tests {
		err := ProcessMessage(test.id, test.value, test.wg, context.TODO())
		assert.Equal(t, stopFn, "stopeks")
		assert.IsType(t, test.err, err)
	}
}
func TestSlsStopMessage(t *testing.T) {
	executorsList = []IExecutor{newSls(), newEks()}
	api = newSdAPI

	var wg sync.WaitGroup
	wg.Add(1)
	tests := []struct {
		id    int
		value string
		wg    *sync.WaitGroup
		err   error
	}{
		{
			id:    1,
			value: "ewogICAgImpvYiI6ICJzdG9wIiwKICAgICJleGVjdXRvclR5cGUiOiAic2xzIiwKICAgICJidWlsZENvbmZpZyI6IHsKICAgICAgICAiYnVpbGRJZCI6IDM1MzIzOSwKICAgICAgICAiam9iSWQiOiA2ODIyLAogICAgICAgICJqb2JTdGF0ZSI6ICJFTkFCTEVEIiwKICAgICAgICAiam9iQXJjaGl2ZWQiOiBmYWxzZSwKICAgICAgICAiam9iTmFtZSI6ICJQUi04OmRlcGxveSIsCiAgICAgICAgImFubm90YXRpb25zIjogewogICAgICAgICAgICAic2NyZXdkcml2ZXIuY2QvZXhlY3V0b3IiOiAic2xzIgogICAgICAgIH0sCiAgICAgICAgImJsb2NrZWRCeSI6IFsKICAgICAgICAgICAgNjgyMgogICAgICAgIF0sCiAgICAgICAgInBpcGVsaW5lIjogewogICAgICAgICAgICAiaWQiOiAxODk4LAogICAgICAgICAgICAic2NtQ29udGV4dCI6ICJnaXRodWI6Z2l0aHViLmNvbSIKICAgICAgICB9LAogICAgICAgICJmcmVlemVXaW5kb3dzIjogW10sCiAgICAgICAgImFwaVVyaSI6ICJodHRwczovL2FwaS5zY3Jld2RyaXZlci5jZCIsCiAgICAgICAgImV2ZW50SWQiOiAzNDY4NjMsCiAgICAgICAgImNvbnRhaW5lciI6ICIxMTExMTExMTEuZGtyLmVjci51cy1lYXN0LTIuYW1hem9uYXdzLmNvbS9zY3Jld2RyaXZlci1odWI6bm9kZTEyIiwKICAgICAgICAicGlwZWxpbmVJZCI6IDE4OTgsCiAgICAgICAgImlzUFIiOiB0cnVlLAogICAgICAgICJwclBhcmVudEpvYklkIjogNjc4OSwKICAgICAgICAicHJvdmlkZXIiOiB7CiAgICAgICAgICAgICJuYW1lIjogImF3cyIsCiAgICAgICAgICAgICJyZWdpb24iOiAidXMtZWFzdC0yIiwKICAgICAgICAgICAgImFjY291bnRJZCI6IDExMTExMTExMSwKICAgICAgICAgICAgInZwYyI6IHsKICAgICAgICAgICAgICAgICJ2cGNJZCI6ICJ2cGMtMDkxNGNhNzFiMWYyNzQxNjciLAogICAgICAgICAgICAgICAgInNlY3VyaXR5R3JvdXBJZHMiOiBbCiAgICAgICAgICAgICAgICAgICAgInNnLTA1ZTQ4MmU2M2ExODAyYWE0IgogICAgICAgICAgICAgICAgXSwKICAgICAgICAgICAgICAgICJzdWJuZXRJZHMiOiBbCiAgICAgICAgICAgICAgICAgICAgInN1Ym5ldC0wYTdiYWVkOGY2MzJkNDFjNiIsCiAgICAgICAgICAgICAgICAgICAgInN1Ym5ldC0wZjIwZTNiNDExOTM2YTAxOSIKICAgICAgICAgICAgICAgIF0KICAgICAgICAgICAgfSwKICAgICAgICAgICAgInJvbGUiOiAiYXJuOmF3czppYW06OjExMTExMTExMTpyb2xlL2NkLnNjcmV3ZHJpdmVyLmNvbnN1bWVyLmludGVncmF0aW9uLXNkYnVpbGQiLAogICAgICAgICAgICAiZXhlY3V0b3IiOiAic2xzIiwKICAgICAgICAgICAgImxhdW5jaGVySW1hZ2UiOiAiMTExMTExMTExLmRrci5lY3IudXMtZWFzdC0yLmFtYXpvbmF3cy5jb20vc2NyZXdkcml2ZXItaHViOmxhdW5jaGVydjYuMC4xNDciLAogICAgICAgICAgICAibGF1bmNoZXJWZXJzaW9uIjogInY2LjAuMTQ3IiwKICAgICAgICAgICAgImV4ZWN1dG9yTG9ncyI6IHRydWUsCiAgICAgICAgICAgICJwcml2aWxlZ2VkTW9kZSI6IGZhbHNlLAogICAgICAgICAgICAiY29tcHV0ZVR5cGUiOiAiQlVJTERfR0VORVJBTDFfU01BTEwiLAogICAgICAgICAgICAiZW52aXJvbm1lbnQiOiAiTElOVVhfQ09OVEFJTkVSIgogICAgICAgIH0sCiAgICAgICAgInRva2VuIjogInNhbXBsZXRva2VuIiwKICAgICAgICAiYnVpbGRUaW1lb3V0IjogOTAsCiAgICAgICAgInVpVXJpIjogImh0dHBzOi8vc2NyZXdkcml2ZXIuY2QiLAogICAgICAgICJzdG9yZVVyaSI6ICJodHRwczovL3N0b3JlLnNjcmV3ZHJpdmVyLmNkIgogICAgfQp9",
			wg:    &wg,
			err:   nil,
		},
	}
	for _, test := range tests {
		err := ProcessMessage(test.id, test.value, test.wg, context.TODO())
		assert.Equal(t, "stopsls", stopSlsFn)
		assert.IsType(t, test.err, err)
	}
}
func TestSlsStartMessage(t *testing.T) {
	executorsList = []IExecutor{newSls(), newEks()}
	api = newSdAPI

	var wg sync.WaitGroup
	wg.Add(1)
	tests := []struct {
		id    int
		value string
		wg    *sync.WaitGroup
		err   error
	}{
		{
			id:    1,
			value: "ewogICAgImpvYiI6ICJzdGFydCIsCiAgICAiZXhlY3V0b3JUeXBlIjogInNscyIsCiAgICAiYnVpbGRDb25maWciOiB7CiAgICAgICAgImpvYklkIjogNjgyMiwKICAgICAgICAiam9iU3RhdGUiOiAiRU5BQkxFRCIsCiAgICAgICAgImpvYkFyY2hpdmVkIjogZmFsc2UsCiAgICAgICAgImpvYk5hbWUiOiAiUFItODpkZXBsb3kiLAogICAgICAgICJhbm5vdGF0aW9ucyI6IHsKICAgICAgICAgICAgInNjcmV3ZHJpdmVyLmNkL2V4ZWN1dG9yIjogInNscyIKICAgICAgICB9LAogICAgICAgICJibG9ja2VkQnkiOiBbCiAgICAgICAgICAgIDY4MjIKICAgICAgICBdLAogICAgICAgICJwaXBlbGluZSI6IHsKICAgICAgICAgICAgImlkIjogMTg5OCwKICAgICAgICAgICAgInNjbUNvbnRleHQiOiAiZ2l0aHViOmdpdGh1Yi5jb20iCiAgICAgICAgfSwKICAgICAgICAiZnJlZXplV2luZG93cyI6IFtdLAogICAgICAgICJhcGlVcmkiOiAiaHR0cHM6Ly9hcGkuc2NyZXdkcml2ZXIuY2QiLAogICAgICAgICJidWlsZElkIjogMzUzMjM5LAogICAgICAgICJldmVudElkIjogMzQ2ODYzLAogICAgICAgICJjb250YWluZXIiOiAiMTExMTExMTExLmRrci5lY3IudXMtZWFzdC0yLmFtYXpvbmF3cy5jb20vc2NyZXdkcml2ZXItaHViOm5vZGUxMiIsCiAgICAgICAgInBpcGVsaW5lSWQiOiAxODk4LAogICAgICAgICJpc1BSIjogdHJ1ZSwKICAgICAgICAicHJQYXJlbnRKb2JJZCI6IDY3ODksCiAgICAgICAgInByb3ZpZGVyIjogewogICAgICAgICAgICAibmFtZSI6ICJhd3MiLAogICAgICAgICAgICAicmVnaW9uIjogInVzLWVhc3QtMiIsCiAgICAgICAgICAgICJhY2NvdW50SWQiOiAxMTExMTExMTEsCiAgICAgICAgICAgICJ2cGMiOiB7CiAgICAgICAgICAgICAgICAidnBjSWQiOiAidnBjLTA5MTRjYTcxYjFmMjc0MTY3IiwKICAgICAgICAgICAgICAgICJzZWN1cml0eUdyb3VwSWRzIjogWwogICAgICAgICAgICAgICAgICAgICJzZy0wNWU0ODJlNjNhMTgwMmFhNCIKICAgICAgICAgICAgICAgIF0sCiAgICAgICAgICAgICAgICAic3VibmV0SWRzIjogWwogICAgICAgICAgICAgICAgICAgICJzdWJuZXQtMGE3YmFlZDhmNjMyZDQxYzYiLAogICAgICAgICAgICAgICAgICAgICJzdWJuZXQtMGYyMGUzYjQxMTkzNmEwMTkiCiAgICAgICAgICAgICAgICBdCiAgICAgICAgICAgIH0sCiAgICAgICAgICAgICJyb2xlIjogImFybjphd3M6aWFtOjoxMTExMTExMTE6cm9sZS9jZC5zY3Jld2RyaXZlci5jb25zdW1lci5pbnRlZ3JhdGlvbi1zZGJ1aWxkIiwKICAgICAgICAgICAgImV4ZWN1dG9yIjogInNscyIsCiAgICAgICAgICAgICJsYXVuY2hlckltYWdlIjogIjExMTExMTExMS5ka3IuZWNyLnVzLWVhc3QtMi5hbWF6b25hd3MuY29tL3NjcmV3ZHJpdmVyLWh1YjpsYXVuY2hlcnY2LjAuMTQ3IiwKICAgICAgICAgICAgImxhdW5jaGVyVmVyc2lvbiI6ICJ2Ni4wLjE0NyIsCiAgICAgICAgICAgICJleGVjdXRvckxvZ3MiOiB0cnVlLAogICAgICAgICAgICAicHJpdmlsZWdlZE1vZGUiOiBmYWxzZSwKICAgICAgICAgICAgImNvbXB1dGVUeXBlIjogIkJVSUxEX0dFTkVSQUwxX1NNQUxMIiwKICAgICAgICAgICAgImVudmlyb25tZW50IjogIkxJTlVYX0NPTlRBSU5FUiIKICAgICAgICB9LAogICAgICAgICJ0b2tlbiI6ICJ0ZXN0dG9rZW4iLAogICAgICAgICJidWlsZFRpbWVvdXQiOiA5MCwKICAgICAgICAidWlVcmkiOiAiaHR0cHM6Ly9zY3Jld2RyaXZlci5jZCIsCiAgICAgICAgInN0b3JlVXJpIjogImh0dHBzOi8vc3RvcmUuc2NyZXdkcml2ZXIuY2QiCiAgICB9Cn0=",
			wg:    &wg,
			err:   nil,
		},
	}
	for _, test := range tests {
		err := ProcessMessage(test.id, test.value, test.wg, context.TODO())
		assert.Equal(t, "startsls", startSlsFn)
		assert.IsType(t, test.err, err)
	}
}
