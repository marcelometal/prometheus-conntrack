// Copyright 2016 conntrack-prometheus authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package collector

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tsuru/prometheus-conntrack/workload"
	workloadTesting "github.com/tsuru/prometheus-conntrack/workload/testing"
)

type fakeConntrack struct {
	calls int
	conns [][]*Conn
}

func (f *fakeConntrack) conntrack() ([]*Conn, error) {
	f.calls = f.calls + 1
	return f.conns[f.calls-1], nil
}

func TestCollector(t *testing.T) {
	conntrack := &fakeConntrack{
		conns: [][]*Conn{
			{
				{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
				{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
				{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.5", ReplyPort: "2376", State: "ESTABLISHED", Protocol: "tcp"},
			},
			{
				{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
			},
			{
				{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
			},
		},
	}

	collector := New(
		workloadTesting.New("containerd", "container", []*workload.Workload{
			{Name: "my-container1", IP: "10.10.1.2", Labels: map[string]string{"label1": "val1", "app": "app1"}},
		}),
		conntrack.conntrack,
		[]string{"app"},
	)
	prometheus.MustRegister(collector)
	rr := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/metrics", nil)
	require.NoError(t, err)
	promhttp.Handler().ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	lines := strings.Split(rr.Body.String(), "\n")
	assert.Contains(t, lines, `conntrack_workload_connections{container="my-container1",destination="192.168.50.4:2375",label_app="app1",protocol="tcp",state="ESTABLISHED"} 2`)
	assert.Contains(t, lines, `conntrack_workload_connections{container="my-container1",destination="192.168.50.5:2376",label_app="app1",protocol="tcp",state="ESTABLISHED"} 1`)

	req, err = http.NewRequest("GET", "/metrics", nil)
	require.NoError(t, err)
	rr = httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	lines = strings.Split(rr.Body.String(), "\n")
	assert.Contains(t, lines, `conntrack_workload_connections{container="my-container1",destination="192.168.50.4:2375",label_app="app1",protocol="tcp",state="ESTABLISHED"} 1`)
	assert.Contains(t, lines, `conntrack_workload_connections{container="my-container1",destination="192.168.50.5:2376",label_app="app1",protocol="tcp",state="ESTABLISHED"} 0`)

	req, err = http.NewRequest("GET", "/metrics", nil)
	require.NoError(t, err)

	rr = httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	lines = strings.Split(rr.Body.String(), "\n")
	assert.Contains(t, lines, `conntrack_workload_connections{container="my-container1",destination="192.168.50.4:2375",label_app="app1",protocol="tcp",state="ESTABLISHED"} 1`)
}

func TestPerformMetricClean(t *testing.T) {
	collector := &ConntrackCollector{}
	now := time.Now().UTC()
	collector.lastUsedTuples.Store(accumulatorKey{workload: "w1", state: "estab", protocol: "tcp", destination: "blah"}, now.Add(time.Minute*-60))
	collector.lastUsedTuples.Store(accumulatorKey{workload: "w2", state: "estab", protocol: "tcp", destination: "blah"}, now)
	collector.lastUsedTuples.Store(accumulatorKey{workload: "w3", state: "estab", protocol: "tcp", destination: "blah"}, now.Add(time.Minute*60))

	collector.performMetricCleaner()

	keys := []string{}
	collector.lastUsedTuples.Range(func(key, lastUsed interface{}) bool {
		keys = append(keys, key.(accumulatorKey).workload)
		return true
	})
	sort.Strings(keys)
	assert.Equal(t, []string{"w2", "w3"}, keys)
}

func BenchmarkCollector(b *testing.B) {
	conns := []*Conn{
		{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.3", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2374", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.6", ReplyPort: "2376", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.3", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.7", ReplyPort: "2376", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.3", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2374", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.6", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.5", ReplyPort: "2376", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.3", OriginPort: "33404", ReplyIP: "192.168.50.5", ReplyPort: "2375", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.2", OriginPort: "33404", ReplyIP: "192.168.50.4", ReplyPort: "2374", State: "ESTABLISHED", Protocol: "tcp"},
		{OriginIP: "10.10.1.1", OriginPort: "33404", ReplyIP: "192.168.50.6", ReplyPort: "2376", State: "ESTABLISHED", Protocol: "tcp"},
	}

	conntrack := func() ([]*Conn, error) {
		return conns, nil
	}
	collector := New(
		workloadTesting.New("containerd", "container", []*workload.Workload{
			{Name: "my-container1", IP: "10.10.1.2"},
			{Name: "my-container2", IP: "10.10.1.3"},
		}),
		conntrack,
		[]string{},
	)
	ch := make(chan prometheus.Metric)
	go func() {
		for _ = range ch {
		}
	}()
	for n := 0; n < b.N; n++ {
		collector.Collect(ch)
	}
	close(ch)
}
