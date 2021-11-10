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
							Value:         "ewogICAgImpvYiI6ICJzdGFydCIsCiAgICAiYnVpbGRDb25maWciOiB7CiAgICAgICAgImpvYklkIjogNDE3NjYsCiAgICAgICAgImpvYlN0YXRlIjogIkVOQUJMRUQiLAogICAgICAgICJqb2JBcmNoaXZlZCI6IGZhbHNlLAogICAgICAgICJqb2JOYW1lIjogImRlcGxveWFwcGVrcyIsCiAgICAgICAgImFubm90YXRpb25zIjogewogICAgICAgICAgICAic2NyZXdkcml2ZXIuY2QvZXhlY3V0b3IiOiAiZWtzIgogICAgICAgIH0sCiAgICAgICAgImJsb2NrZWRCeSI6IFsKICAgICAgICAgICAgNDE3NjYKICAgICAgICBdLAogICAgICAgICJwaXBlbGluZSI6IHsKICAgICAgICAgICAgImlkIjogOTY2OCwKICAgICAgICAgICAgInNjbUNvbnRleHQiOiAiZ2l0aHViOmdpdGh1Yi5jb20iCiAgICAgICAgfSwKICAgICAgICAiZnJlZXplV2luZG93cyI6IFtdLAogICAgICAgICJhcGlVcmkiOiAiaHR0cHM6Ly9iZXRhLmFwaS5zY3Jld2RyaXZlci5jZCIsCiAgICAgICAgImJ1aWxkSWQiOiAxMjY2NzUsCiAgICAgICAgImV2ZW50SWQiOiA4MjE5MSwKICAgICAgICAiY29udGFpbmVyIjogImNlbnRvczpjZW50b3M3IiwKICAgICAgICAicGlwZWxpbmVJZCI6IDk2NjgsCiAgICAgICAgImlzUFIiOiBmYWxzZSwKICAgICAgICAidG9rZW4iOiAiZXlKaGJHY2lPaUpTVXpJMU5pSXNJblI1Y0NJNklrcFhWQ0o5LmV5SjFjMlZ5Ym1GdFpTSTZNVEkyTmpjMUxDSmlkV2xzWkVsa0lqb3hNalkyTnpVc0ltcHZZa2xrSWpvME1UYzJOaXdpWlhabGJuUkpaQ0k2T0RJeE9URXNJbWx6VUZJaU9tWmhiSE5sTENKd2FYQmxiR2x1WlVsa0lqbzVOalk0TENKelkyMURiMjUwWlhoMElqb2laMmwwYUhWaU9tZHBkR2gxWWk1amIyMGlMQ0p6WTI5d1pTSTZXeUowWlcxd2IzSmhiQ0pkTENKcFlYUWlPakUyTXpBNU1EYzRNallzSW1WNGNDSTZNVFl6TURrMU1UQXlOaXdpYW5ScElqb2laRFkyTURFNFlXWXRZak5rTkMwME5tRTFMV0kyTmpBdFptWXhOV013Tm1Gak5UZGxJbjAuaWoydldKQnJ5YmtjRDlXVnJObGxlRjViUU8yVlhwWnMwTnQxcXdsZkZoYzFwZ1dQVEMySVdNeDBCUlBhWHpOVFJxMzliVlhDbUgxb0FSSTNxQVdJU3ZGTVhONEt0NHM5aXhjWGxVUmNVbUFwTTZyczkwMTQ2ZVNwdmJzQW1nSGJ0cnE5VW05d0s2d2tNOGhzQ1h0Y2pwQWVtRUtHMk52emR4dnJiVVg2RElGQVlRYjFVc0ViQ1ZDaFFWazJkblF2YnhBaGpYNFhnajYzUmNyRXFLbzhLREFsWWNOLTBJNmNsTDFPekZUcTJ4dS1rR2FENm5fWUlZMEprcjJFaWJQV0M3OGw4d2JEdGVZczRWaEdBYS16MTRST1RRSnBJYWQ5SmFtRXY4bGlYR2FrZ2tzMnprTFduQ2xjWGlxY3QxRm5vcnFlUDZveEtSbzRlT3laM1NJLWZ3IiwKICAgICAgICAic3VibmV0cyI6IFsKICAgICAgICAgICAgInN1Ym5ldC0wOTYxMzRlZmEwNGJmNGM0NyIsCiAgICAgICAgICAgICJzdWJuZXQtMDNlMTgxOTgyYjQ1NDc2NzUiLAogICAgICAgICAgICAic3VibmV0LTA3ZWY1YzZjODA4ZGE2OTNkIgogICAgICAgIF0sCiAgICAgICAgInNlY3VyaXR5R3JvdXBJZHMiOiBbCiAgICAgICAgICAgICJzZy0wZWRkYjcxMzI2NDY3ZmRhMyIKICAgICAgICBdLAogICAgICAgICJ2cGNJZCI6ICJ2cGMtMDQ4NDYxM2EzNDhkMmFkNDEiLAogICAgICAgICJyb2xlQXJuIjogImFybjphd3M6aWFtOjo1NTE5NjQzMzczMDI6cm9sZS9zZXJ2aWNlLXJvbGUvY29kZWJ1aWxkLXNsc2J1aWxkLTEwMS1zZXJ2aWNlLXJvbGUiLAogICAgICAgICJlbnF1ZXVlVGltZSI6ICIyMDIxLTA5LTA2VDA1OjU3OjA4LjAwMVoiLAogICAgICAgICJidWlsZFRpbWVvdXQiOiA2MCwKICAgICAgICAic3RvcmVVcmkiOiAiaHR0cHM6Ly9iZXRhLnN0b3JlLnNjcmV3ZHJpdmVyLmNkIiwKICAgICAgICAidWlVcmkiOiAiaHR0cHM6Ly9iZXRhLmNkLnNjcmV3ZHJpdmVyLmNkIiwKICAgICAgICAibGF1bmNoZXJJbWFnZSI6ICI1NTE5NjQzMzczMDIuZGtyLmVjci51cy13ZXN0LTIuYW1hem9uYXdzLmNvbS9zZC1odWI6c2NyZXdkcml2ZXJjZC1sYXVuY2hlciIsCiAgICAgICAgImxhdW5jaGVyVmVyc2lvbiI6ICJ2Ni4wLjEzNyIsCiAgICAgICAgImxhdW5jaGVyQ29tcHV0ZVR5cGUiOiAiQlVJTERfR0VORVJBTDFfTUVESVVNIiwKICAgICAgICAiY29tcHV0ZVR5cGUiOiAiQlVJTERfR0VORVJBTDFfTUVESVVNIiwKICAgICAgICAibG9nc0VuYWJsZWQiOiB0cnVlLAogICAgICAgICJ0b3BpYyI6ICJidWlsZHMtNTUxOTY0MzM3MzAyLXVzdzIiLAogICAgICAgICJkbGMiOiB0cnVlLAogICAgICAgICJwcnVuZSI6IHRydWUsCiAgICAgICAgImNsdXN0ZXJOYW1lIjogInNkLWJ1aWxkLWVrcyIsCiAgICAgICAgInByZWZpeCI6ICJiZXRhIiwKICAgICAgICAiY3B1TGltaXQiOiAiMjAwMG0iLAogICAgICAgICJtZW1vcnlMaW1pdCI6ICIyR2kiCiAgICB9LAogICAgImV4ZWN1dG9yVHlwZSI6ICJla3MiCn0=",
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
			value: "ewogICAgImpvYiI6ICJzdGFydCIsCiAgICAiYnVpbGRDb25maWciOiB7CiAgICAgICAgImpvYklkIjogNDE3NjYsCiAgICAgICAgImpvYlN0YXRlIjogIkVOQUJMRUQiLAogICAgICAgICJqb2JBcmNoaXZlZCI6IGZhbHNlLAogICAgICAgICJqb2JOYW1lIjogImRlcGxveWFwcGVrcyIsCiAgICAgICAgImFubm90YXRpb25zIjogewogICAgICAgICAgICAic2NyZXdkcml2ZXIuY2QvZXhlY3V0b3IiOiAiZWtzIgogICAgICAgIH0sCiAgICAgICAgImJsb2NrZWRCeSI6IFsKICAgICAgICAgICAgNDE3NjYKICAgICAgICBdLAogICAgICAgICJwaXBlbGluZSI6IHsKICAgICAgICAgICAgImlkIjogOTY2OCwKICAgICAgICAgICAgInNjbUNvbnRleHQiOiAiZ2l0aHViOmdpdGh1Yi5jb20iCiAgICAgICAgfSwKICAgICAgICAiZnJlZXplV2luZG93cyI6IFtdLAogICAgICAgICJhcGlVcmkiOiAiaHR0cHM6Ly9iZXRhLmFwaS5zY3Jld2RyaXZlci5jZCIsCiAgICAgICAgImJ1aWxkSWQiOiAxMjY2NzUsCiAgICAgICAgImV2ZW50SWQiOiA4MjE5MSwKICAgICAgICAiY29udGFpbmVyIjogImNlbnRvczpjZW50b3M3IiwKICAgICAgICAicGlwZWxpbmVJZCI6IDk2NjgsCiAgICAgICAgImlzUFIiOiBmYWxzZSwKICAgICAgICAidG9rZW4iOiAiZXlKaGJHY2lPaUpTVXpJMU5pSXNJblI1Y0NJNklrcFhWQ0o5LmV5SjFjMlZ5Ym1GdFpTSTZNVEkyTmpjMUxDSmlkV2xzWkVsa0lqb3hNalkyTnpVc0ltcHZZa2xrSWpvME1UYzJOaXdpWlhabGJuUkpaQ0k2T0RJeE9URXNJbWx6VUZJaU9tWmhiSE5sTENKd2FYQmxiR2x1WlVsa0lqbzVOalk0TENKelkyMURiMjUwWlhoMElqb2laMmwwYUhWaU9tZHBkR2gxWWk1amIyMGlMQ0p6WTI5d1pTSTZXeUowWlcxd2IzSmhiQ0pkTENKcFlYUWlPakUyTXpBNU1EYzRNallzSW1WNGNDSTZNVFl6TURrMU1UQXlOaXdpYW5ScElqb2laRFkyTURFNFlXWXRZak5rTkMwME5tRTFMV0kyTmpBdFptWXhOV013Tm1Gak5UZGxJbjAuaWoydldKQnJ5YmtjRDlXVnJObGxlRjViUU8yVlhwWnMwTnQxcXdsZkZoYzFwZ1dQVEMySVdNeDBCUlBhWHpOVFJxMzliVlhDbUgxb0FSSTNxQVdJU3ZGTVhONEt0NHM5aXhjWGxVUmNVbUFwTTZyczkwMTQ2ZVNwdmJzQW1nSGJ0cnE5VW05d0s2d2tNOGhzQ1h0Y2pwQWVtRUtHMk52emR4dnJiVVg2RElGQVlRYjFVc0ViQ1ZDaFFWazJkblF2YnhBaGpYNFhnajYzUmNyRXFLbzhLREFsWWNOLTBJNmNsTDFPekZUcTJ4dS1rR2FENm5fWUlZMEprcjJFaWJQV0M3OGw4d2JEdGVZczRWaEdBYS16MTRST1RRSnBJYWQ5SmFtRXY4bGlYR2FrZ2tzMnprTFduQ2xjWGlxY3QxRm5vcnFlUDZveEtSbzRlT3laM1NJLWZ3IiwKICAgICAgICAic3VibmV0cyI6IFsKICAgICAgICAgICAgInN1Ym5ldC0wOTYxMzRlZmEwNGJmNGM0NyIsCiAgICAgICAgICAgICJzdWJuZXQtMDNlMTgxOTgyYjQ1NDc2NzUiLAogICAgICAgICAgICAic3VibmV0LTA3ZWY1YzZjODA4ZGE2OTNkIgogICAgICAgIF0sCiAgICAgICAgInNlY3VyaXR5R3JvdXBJZHMiOiBbCiAgICAgICAgICAgICJzZy0wZWRkYjcxMzI2NDY3ZmRhMyIKICAgICAgICBdLAogICAgICAgICJ2cGNJZCI6ICJ2cGMtMDQ4NDYxM2EzNDhkMmFkNDEiLAogICAgICAgICJyb2xlQXJuIjogImFybjphd3M6aWFtOjo1NTE5NjQzMzczMDI6cm9sZS9zZXJ2aWNlLXJvbGUvY29kZWJ1aWxkLXNsc2J1aWxkLTEwMS1zZXJ2aWNlLXJvbGUiLAogICAgICAgICJlbnF1ZXVlVGltZSI6ICIyMDIxLTA5LTA2VDA1OjU3OjA4LjAwMVoiLAogICAgICAgICJidWlsZFRpbWVvdXQiOiA2MCwKICAgICAgICAic3RvcmVVcmkiOiAiaHR0cHM6Ly9iZXRhLnN0b3JlLnNjcmV3ZHJpdmVyLmNkIiwKICAgICAgICAidWlVcmkiOiAiaHR0cHM6Ly9iZXRhLmNkLnNjcmV3ZHJpdmVyLmNkIiwKICAgICAgICAibGF1bmNoZXJJbWFnZSI6ICI1NTE5NjQzMzczMDIuZGtyLmVjci51cy13ZXN0LTIuYW1hem9uYXdzLmNvbS9zZC1odWI6c2NyZXdkcml2ZXJjZC1sYXVuY2hlciIsCiAgICAgICAgImxhdW5jaGVyVmVyc2lvbiI6ICJ2Ni4wLjEzNyIsCiAgICAgICAgImxhdW5jaGVyQ29tcHV0ZVR5cGUiOiAiQlVJTERfR0VORVJBTDFfTUVESVVNIiwKICAgICAgICAiY29tcHV0ZVR5cGUiOiAiQlVJTERfR0VORVJBTDFfTUVESVVNIiwKICAgICAgICAibG9nc0VuYWJsZWQiOiB0cnVlLAogICAgICAgICJ0b3BpYyI6ICJidWlsZHMtNTUxOTY0MzM3MzAyLXVzdzIiLAogICAgICAgICJkbGMiOiB0cnVlLAogICAgICAgICJwcnVuZSI6IHRydWUsCiAgICAgICAgImNsdXN0ZXJOYW1lIjogInNkLWJ1aWxkLWVrcyIsCiAgICAgICAgInByZWZpeCI6ICJiZXRhIiwKICAgICAgICAiY3B1TGltaXQiOiAiMjAwMG0iLAogICAgICAgICJtZW1vcnlMaW1pdCI6ICIyR2kiCiAgICB9LAogICAgImV4ZWN1dG9yVHlwZSI6ICJla3MiCn0=",
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
			value: "ewogICAgImpvYiI6ICJzdG9wIiwKICAgICJidWlsZENvbmZpZyI6IHsKICAgICAgICAiam9iSWQiOiA0MTc2NiwKICAgICAgICAiam9iU3RhdGUiOiAiRU5BQkxFRCIsCiAgICAgICAgImpvYkFyY2hpdmVkIjogZmFsc2UsCiAgICAgICAgImpvYk5hbWUiOiAiZGVwbG95YXBwZWtzIiwKICAgICAgICAiYW5ub3RhdGlvbnMiOiB7CiAgICAgICAgICAgICJzY3Jld2RyaXZlci5jZC9leGVjdXRvciI6ICJla3MiCiAgICAgICAgfSwKICAgICAgICAiYmxvY2tlZEJ5IjogWwogICAgICAgICAgICA0MTc2NgogICAgICAgIF0sCiAgICAgICAgInBpcGVsaW5lIjogewogICAgICAgICAgICAiaWQiOiA5NjY4LAogICAgICAgICAgICAic2NtQ29udGV4dCI6ICJnaXRodWI6Z2l0aHViLmNvbSIKICAgICAgICB9LAogICAgICAgICJmcmVlemVXaW5kb3dzIjogW10sCiAgICAgICAgImFwaVVyaSI6ICJodHRwczovL2JldGEuYXBpLnNjcmV3ZHJpdmVyLmNkIiwKICAgICAgICAiYnVpbGRJZCI6IDEyNjY3NSwKICAgICAgICAiZXZlbnRJZCI6IDgyMTkxLAogICAgICAgICJjb250YWluZXIiOiAiY2VudG9zOmNlbnRvczciLAogICAgICAgICJwaXBlbGluZUlkIjogOTY2OCwKICAgICAgICAiaXNQUiI6IGZhbHNlLAogICAgICAgICJ0b2tlbiI6ICJleUpoYkdjaU9pSlNVekkxTmlJc0luUjVjQ0k2SWtwWFZDSjkuZXlKMWMyVnlibUZ0WlNJNk1USTJOamMxTENKaWRXbHNaRWxrSWpveE1qWTJOelVzSW1wdllrbGtJam8wTVRjMk5pd2laWFpsYm5SSlpDSTZPREl4T1RFc0ltbHpVRklpT21aaGJITmxMQ0p3YVhCbGJHbHVaVWxrSWpvNU5qWTRMQ0p6WTIxRGIyNTBaWGgwSWpvaVoybDBhSFZpT21kcGRHaDFZaTVqYjIwaUxDSnpZMjl3WlNJNld5SjBaVzF3YjNKaGJDSmRMQ0pwWVhRaU9qRTJNekE1TURjNE1qWXNJbVY0Y0NJNk1UWXpNRGsxTVRBeU5pd2lhblJwSWpvaVpEWTJNREU0WVdZdFlqTmtOQzAwTm1FMUxXSTJOakF0Wm1ZeE5XTXdObUZqTlRkbEluMC5pajJ2V0pCcnlia2NEOVdWck5sbGVGNWJRTzJWWHBaczBOdDFxd2xmRmhjMXBnV1BUQzJJV014MEJSUGFYek5UUnEzOWJWWENtSDFvQVJJM3FBV0lTdkZNWE40S3Q0czlpeGNYbFVSY1VtQXBNNnJzOTAxNDZlU3B2YnNBbWdIYnRycTlVbTl3SzZ3a004aHNDWHRjanBBZW1FS0cyTnZ6ZHh2cmJVWDZESUZBWVFiMVVzRWJDVkNoUVZrMmRuUXZieEFoalg0WGdqNjNSY3JFcUtvOEtEQWxZY04tMEk2Y2xMMU96RlRxMnh1LWtHYUQ2bl9ZSVkwSmtyMkVpYlBXQzc4bDh3YkR0ZVlzNFZoR0FhLXoxNFJPVFFKcElhZDlKYW1FdjhsaVhHYWtna3MyemtMV25DbGNYaXFjdDFGbm9ycWVQNm94S1JvNGVPeVozU0ktZnciLAogICAgICAgICJzdWJuZXRzIjogWwogICAgICAgICAgICAic3VibmV0LTA5NjEzNGVmYTA0YmY0YzQ3IiwKICAgICAgICAgICAgInN1Ym5ldC0wM2UxODE5ODJiNDU0NzY3NSIsCiAgICAgICAgICAgICJzdWJuZXQtMDdlZjVjNmM4MDhkYTY5M2QiCiAgICAgICAgXSwKICAgICAgICAic2VjdXJpdHlHcm91cElkcyI6IFsKICAgICAgICAgICAgInNnLTBlZGRiNzEzMjY0NjdmZGEzIgogICAgICAgIF0sCiAgICAgICAgInZwY0lkIjogInZwYy0wNDg0NjEzYTM0OGQyYWQ0MSIsCiAgICAgICAgInJvbGVBcm4iOiAiYXJuOmF3czppYW06OjU1MTk2NDMzNzMwMjpyb2xlL3NlcnZpY2Utcm9sZS9jb2RlYnVpbGQtc2xzYnVpbGQtMTAxLXNlcnZpY2Utcm9sZSIsCiAgICAgICAgImVucXVldWVUaW1lIjogIjIwMjEtMDktMDZUMDU6NTc6MDguMDAxWiIsCiAgICAgICAgImJ1aWxkVGltZW91dCI6IDYwLAogICAgICAgICJzdG9yZVVyaSI6ICJodHRwczovL2JldGEuc3RvcmUuc2NyZXdkcml2ZXIuY2QiLAogICAgICAgICJ1aVVyaSI6ICJodHRwczovL2JldGEuY2Quc2NyZXdkcml2ZXIuY2QiLAogICAgICAgICJsYXVuY2hlckltYWdlIjogIjU1MTk2NDMzNzMwMi5ka3IuZWNyLnVzLXdlc3QtMi5hbWF6b25hd3MuY29tL3NkLWh1YjpzY3Jld2RyaXZlcmNkLWxhdW5jaGVyIiwKICAgICAgICAibGF1bmNoZXJWZXJzaW9uIjogInY2LjAuMTM3IiwKICAgICAgICAibGF1bmNoZXJDb21wdXRlVHlwZSI6ICJCVUlMRF9HRU5FUkFMMV9NRURJVU0iLAogICAgICAgICJjb21wdXRlVHlwZSI6ICJCVUlMRF9HRU5FUkFMMV9NRURJVU0iLAogICAgICAgICJsb2dzRW5hYmxlZCI6IHRydWUsCiAgICAgICAgInRvcGljIjogImJ1aWxkcy01NTE5NjQzMzczMDItdXN3MiIsCiAgICAgICAgImRsYyI6IHRydWUsCiAgICAgICAgInBydW5lIjogdHJ1ZSwKICAgICAgICAiY2x1c3Rlck5hbWUiOiAic2QtYnVpbGQtZWtzIiwKICAgICAgICAicHJlZml4IjogImJldGEiLAogICAgICAgICJjcHVMaW1pdCI6ICIyMDAwbSIsCiAgICAgICAgIm1lbW9yeUxpbWl0IjogIjJHaSIKICAgIH0sCiAgICAiZXhlY3V0b3JUeXBlIjogImVrcyIKfQ==",
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
			value: "eyJqb2IiOiJzdG9wIiwiYnVpbGRDb25maWciOnsiYnVpbGRJZCI6MTI3MTEzLCJqb2JJZCI6NDE3NjUsInRva2VuIjoiZXlKaGJHY2lPaUpTVXpJMU5pSXNJblI1Y0NJNklrcFhWQ0o5LmV5SjFjMlZ5Ym1GdFpTSTZNVEkzTVRFekxDSmlkV2xzWkVsa0lqb3hNamN4TVRNc0ltcHZZa2xrSWpvME1UYzJOU3dpWlhabGJuUkpaQ0k2T0RJME9ETXNJbWx6VUZJaU9tWmhiSE5sTENKd2FYQmxiR2x1WlVsa0lqbzVOalk0TENKelkyMURiMjUwWlhoMElqb2laMmwwYUhWaU9tZHBkR2gxWWk1amIyMGlMQ0p6WTI5d1pTSTZXeUowWlcxd2IzSmhiQ0pkTENKcFlYUWlPakUyTXpNME5Ua3dPVEFzSW1WNGNDSTZNVFl6TXpVd01qSTVNQ3dpYW5ScElqb2lObUUyWmpWaFl6TXRNVGt3WWkwMFl6azBMVGxsWVRZdE5HRXpNbVZoT0dZelpUY3dJbjAuQVRnY0tmcDlveTdqSVVPYnhtT2JUNVZFMnNrYWlJaUVxckRSa0h1Um9UV2k4ZV9NbFdSYjh3ZGN2aENBVklFMTN3SnRyZzFOUnN0MS1jNmw1ejVmSWZKTGE3NzlvcjctelN6OGxRd2ZlYk51MUcyVURNZFRDaWdYLUV5bUFmQllTUVI0LU90WnBMc1hmVmotaTRQd1M2ck1sdzd4SkZKd2JRdHl5R3pJbV8xT2lmSXpTT0ZkaWY3VHRnZnc2NGVGUmU4NUJLYnNVcEZjd1RzOGhOanMxM0NNR2V3UWJ5blhRcmR4dFFmN3BtTlZGSjZIMG0wSThhU2RDNHZYbFNtYXJ2SElXX0hQbHpZQTl6aDNfYjkyS1BKd3B5SUFLc1ZDQkl5TlUwSWVXTjBQMWpTYTdGUVQ2aFBMZXo0VzhPS25FQzlPcWlQQUdEY2ZZcHZnSnlqbkNnIiwiam9iTmFtZSI6ImRlcGxveWFwcHNscyIsImNvbnRhaW5lciI6IjU1MTk2NDMzNzMwMi5ka3IuZWNyLnVzLXdlc3QtMi5hbWF6b25hd3MuY29tL3NkLWh1YjpjZW50b3M3IiwiYXBpVXJpIjoiaHR0cHM6Ly9iZXRhLmFwaS5zY3Jld2RyaXZlci5jZCIsImFubm90YXRpb25zIjp7InNjcmV3ZHJpdmVyLmNkL2V4ZWN1dG9yIjoic2xzIn0sInN1Ym5ldHMiOlsic3VibmV0LTA5NjEzNGVmYTA0YmY0YzQ3Iiwic3VibmV0LTAzZTE4MTk4MmI0NTQ3Njc1Iiwic3VibmV0LTA3ZWY1YzZjODA4ZGE2OTNkIl0sInNlY3VyaXR5R3JvdXBJZHMiOlsic2ctMGVkZGI3MTMyNjQ2N2ZkYTMiXSwidnBjSWQiOiJ2cGMtMDQ4NDYxM2EzNDhkMmFkNDEiLCJyb2xlQXJuIjoiYXJuOmF3czppYW06OjU1MTk2NDMzNzMwMjpyb2xlL3NlcnZpY2Utcm9sZS9jb2RlYnVpbGQtc2xzYnVpbGQtMTAxLXNlcnZpY2Utcm9sZSIsImVucXVldWVUaW1lIjoiMjAyMS0xMC0wNVQxODo0MzowNC4yOTNaIiwiYnVpbGRUaW1lb3V0Ijo2MCwic3RvcmVVcmkiOiJodHRwczovL2JldGEuc3RvcmUuc2NyZXdkcml2ZXIuY2QiLCJ1aVVyaSI6Imh0dHBzOi8vYmV0YS5jZC5zY3Jld2RyaXZlci5jZCIsImxhdW5jaGVySW1hZ2UiOiI1NTE5NjQzMzczMDIuZGtyLmVjci51cy13ZXN0LTIuYW1hem9uYXdzLmNvbS9zZC1odWI6c2NyZXdkcml2ZXJjZC1sYXVuY2hlciIsImxhdW5jaGVyVmVyc2lvbiI6InY2LjAuMTM3IiwibGF1bmNoZXJDb21wdXRlVHlwZSI6IkJVSUxEX0dFTkVSQUwxX01FRElVTSIsImNvbXB1dGVUeXBlIjoiQlVJTERfR0VORVJBTDFfTUVESVVNIiwibG9nc0VuYWJsZWQiOnRydWUsInRvcGljIjoiYnVpbGRzLTU1MTk2NDMzNzMwMi11c3cyIiwiZGxjIjp0cnVlLCJwcnVuZSI6dHJ1ZSwiY2x1c3Rlck5hbWUiOiJzZC1idWlsZC1la3MiLCJwcmVmaXgiOiJiZXRhIiwiY3B1TGltaXQiOiIybSIsIm1lbW9yeUxpbWl0IjoiMkdpIn0sImV4ZWN1dG9yVHlwZSI6InNscyJ9",
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
			value: "eyJqb2IiOiJzdGFydCIsImJ1aWxkQ29uZmlnIjp7ImpvYklkIjo0MTc2NSwiam9iU3RhdGUiOiJFTkFCTEVEIiwiam9iQXJjaGl2ZWQiOmZhbHNlLCJqb2JOYW1lIjoiZGVwbG95YXBwc2xzIiwiYW5ub3RhdGlvbnMiOnsic2NyZXdkcml2ZXIuY2QvZXhlY3V0b3IiOiJzbHMifSwiYmxvY2tlZEJ5IjpbNDE3NjVdLCJwaXBlbGluZSI6eyJpZCI6OTY2OCwic2NtQ29udGV4dCI6ImdpdGh1YjpnaXRodWIuY29tIn0sImZyZWV6ZVdpbmRvd3MiOltdLCJhcGlVcmkiOiJodHRwczovL2JldGEuYXBpLnNjcmV3ZHJpdmVyLmNkIiwiYnVpbGRJZCI6MTI3MTEzLCJldmVudElkIjo4MjQ4MywiY29udGFpbmVyIjoiNTUxOTY0MzM3MzAyLmRrci5lY3IudXMtd2VzdC0yLmFtYXpvbmF3cy5jb20vc2QtaHViOmNlbnRvczciLCJwaXBlbGluZUlkIjo5NjY4LCJpc1BSIjpmYWxzZSwidG9rZW4iOiJleUpoYkdjaU9pSlNVekkxTmlJc0luUjVjQ0k2SWtwWFZDSjkuZXlKMWMyVnlibUZ0WlNJNk1USTNNVEV6TENKaWRXbHNaRWxrSWpveE1qY3hNVE1zSW1wdllrbGtJam8wTVRjMk5Td2laWFpsYm5SSlpDSTZPREkwT0RNc0ltbHpVRklpT21aaGJITmxMQ0p3YVhCbGJHbHVaVWxrSWpvNU5qWTRMQ0p6WTIxRGIyNTBaWGgwSWpvaVoybDBhSFZpT21kcGRHaDFZaTVqYjIwaUxDSnpZMjl3WlNJNld5SjBaVzF3YjNKaGJDSmRMQ0pwWVhRaU9qRTJNek0wTlRrd09UQXNJbVY0Y0NJNk1UWXpNelV3TWpJNU1Dd2lhblJwSWpvaU5tRTJaalZoWXpNdE1Ua3dZaTAwWXprMExUbGxZVFl0TkdFek1tVmhPR1l6WlRjd0luMC5BVGdjS2ZwOW95N2pJVU9ieG1PYlQ1VkUyc2thaUlpRXFyRFJrSHVSb1RXaThlX01sV1JiOHdkY3ZoQ0FWSUUxM3dKdHJnMU5Sc3QxLWM2bDV6NWZJZkpMYTc3OW9yNy16U3o4bFF3ZmViTnUxRzJVRE1kVENpZ1gtRXltQWZCWVNRUjQtT3RacExzWGZWai1pNFB3UzZyTWx3N3hKRkp3YlF0eXlHekltXzFPaWZJelNPRmRpZjdUdGdmdzY0ZUZSZTg1Qktic1VwRmN3VHM4aE5qczEzQ01HZXdRYnluWFFyZHh0UWY3cG1OVkZKNkgwbTBJOGFTZEM0dlhsU21hcnZISVdfSFBsellBOXpoM19iOTJLUEp3cHlJQUtzVkNCSXlOVTBJZVdOMFAxalNhN0ZRVDZoUExlejRXOE9LbkVDOU9xaVBBR0RjZllwdmdKeWpuQ2ciLCJzdWJuZXRzIjpbInN1Ym5ldC0wOTYxMzRlZmEwNGJmNGM0NyIsInN1Ym5ldC0wM2UxODE5ODJiNDU0NzY3NSIsInN1Ym5ldC0wN2VmNWM2YzgwOGRhNjkzZCJdLCJzZWN1cml0eUdyb3VwSWRzIjpbInNnLTBlZGRiNzEzMjY0NjdmZGEzIl0sInZwY0lkIjoidnBjLTA0ODQ2MTNhMzQ4ZDJhZDQxIiwicm9sZUFybiI6ImFybjphd3M6aWFtOjo1NTE5NjQzMzczMDI6cm9sZS9zZXJ2aWNlLXJvbGUvY29kZWJ1aWxkLXNsc2J1aWxkLTEwMS1zZXJ2aWNlLXJvbGUiLCJlbnF1ZXVlVGltZSI6IjIwMjEtMTAtMDVUMTg6Mzg6MTMuOTExWiIsImJ1aWxkVGltZW91dCI6NjAsInN0b3JlVXJpIjoiaHR0cHM6Ly9iZXRhLnN0b3JlLnNjcmV3ZHJpdmVyLmNkIiwidWlVcmkiOiJodHRwczovL2JldGEuY2Quc2NyZXdkcml2ZXIuY2QiLCJsYXVuY2hlckltYWdlIjoiNTUxOTY0MzM3MzAyLmRrci5lY3IudXMtd2VzdC0yLmFtYXpvbmF3cy5jb20vc2QtaHViOnNjcmV3ZHJpdmVyY2QtbGF1bmNoZXIiLCJsYXVuY2hlclZlcnNpb24iOiJ2Ni4wLjEzNyIsImxhdW5jaGVyQ29tcHV0ZVR5cGUiOiJCVUlMRF9HRU5FUkFMMV9NRURJVU0iLCJjb21wdXRlVHlwZSI6IkJVSUxEX0dFTkVSQUwxX01FRElVTSIsImxvZ3NFbmFibGVkIjp0cnVlLCJ0b3BpYyI6ImJ1aWxkcy01NTE5NjQzMzczMDItdXN3MiIsImRsYyI6dHJ1ZSwicHJ1bmUiOnRydWUsImNsdXN0ZXJOYW1lIjoic2QtYnVpbGQtZWtzIiwicHJlZml4IjoiYmV0YSIsImNwdUxpbWl0IjoiMm0iLCJtZW1vcnlMaW1pdCI6IjJHaSJ9LCJleGVjdXRvclR5cGUiOiJzbHMifQ==",
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
