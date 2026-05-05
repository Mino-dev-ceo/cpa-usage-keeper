package cpa

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestRedisQueueClientPopsBatch(t *testing.T) {
	logs := captureRedisQueueClientInfoLogs(t)
	server := newRedisQueueTestServer(t, func(t *testing.T, conn net.Conn) {
		reader := bufio.NewReader(conn)
		if got := readRESPCommand(t, reader); strings.Join(got, " ") != cpaManagementRedisAuthCommand+" secret" {
			t.Fatalf("unexpected auth command: %v", got)
		}
		fmt.Fprint(conn, "+OK\r\n")
		if got := readRESPCommand(t, reader); strings.Join(got, " ") != cpaManagementRedisPopCommand+" "+ManagementUsageQueueKey+" 2" {
			t.Fatalf("unexpected pop command: %v", got)
		}
		fmt.Fprint(conn, "*2\r\n$7\r\n{\"a\":1}\r\n$7\r\n{\"b\":2}\r\n")
	})

	client := NewRedisQueueClient(server.URL, "", "secret", time.Second, ManagementUsageQueueKey, 2)
	messages, err := client.PopUsage(ctxWithTimeout(t))
	if err != nil {
		t.Fatalf("PopUsage returned error: %v", err)
	}

	if len(messages) != 2 || messages[0] != `{"a":1}` || messages[1] != `{"b":2}` {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	if !strings.Contains(logs.String(), `msg="usage queue sync used redis protocol"`) {
		t.Fatalf("expected redis protocol log, got %q", logs.String())
	}
}

func TestRedisQueueClientFallsBackToHTTPUsageQueueWhenRedisFails(t *testing.T) {
	logs := captureRedisQueueClientInfoLogs(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != cpaManagementUsageQueueEndpoint {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("count"); got != "2" {
			t.Fatalf("expected count=2, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("expected management Authorization header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"a":1},{"b":2}]`))
	}))
	defer server.Close()

	client := NewRedisQueueClient(server.URL, "127.0.0.1:1", "secret", 10*time.Millisecond, ManagementUsageQueueKey, 2)
	client.httpClient.httpClient = server.Client()
	messages, err := client.PopUsage(ctxWithTimeout(t))
	if err != nil {
		t.Fatalf("PopUsage returned error: %v", err)
	}
	if len(messages) != 2 || messages[0] != `{"a":1}` || messages[1] != `{"b":2}` {
		t.Fatalf("unexpected messages: %#v", messages)
	}
	content := logs.String()
	if !strings.Contains(content, `msg="usage queue sync used http protocol"`) {
		t.Fatalf("expected http fallback log, got %q", content)
	}
	if !strings.Contains(content, "redis_error=") {
		t.Fatalf("expected redis error field in fallback log, got %q", content)
	}
}

func TestRedisQueueClientCachesHTTPFallbackModeAfterFirstSuccessfulFallback(t *testing.T) {
	logs := captureRedisQueueClientInfoLogs(t)
	var httpCalls atomic.Int32
	httpServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalls.Add(1)
		_, _ = w.Write([]byte(`[{"h":1}]`))
	}))
	defer httpServer.Close()

	client := NewRedisQueueClient(httpServer.URL, "127.0.0.1:1", "secret", 10*time.Millisecond, ManagementUsageQueueKey, 1)
	client.httpClient.httpClient = httpServer.Client()

	for range 2 {
		messages, err := client.PopUsage(ctxWithTimeout(t))
		if err != nil {
			t.Fatalf("PopUsage returned error: %v", err)
		}
		if len(messages) != 1 || messages[0] != `{"h":1}` {
			t.Fatalf("unexpected messages: %#v", messages)
		}
	}

	if httpCalls.Load() != 2 {
		t.Fatalf("expected cached http mode to make two http calls, got %d", httpCalls.Load())
	}
	if count := strings.Count(logs.String(), `msg="usage queue sync used http protocol"`); count != 1 {
		t.Fatalf("expected fallback mode to be logged once, got %d logs: %q", count, logs.String())
	}
}

func TestRedisQueueClientCachesRedisModeAfterFirstSuccessfulPop(t *testing.T) {
	logs := captureRedisQueueClientInfoLogs(t)
	var redisCalls atomic.Int32
	redisServer := newRedisQueueMultiTestServer(t, 2, func(t *testing.T, conn net.Conn) {
		redisCalls.Add(1)
		reader := bufio.NewReader(conn)
		readRESPCommand(t, reader)
		fmt.Fprint(conn, "+OK\r\n")
		readRESPCommand(t, reader)
		fmt.Fprint(conn, "*1\r\n$7\r\n{\"r\":1}\r\n")
	})

	client := NewRedisQueueClient(redisServer.URL, "", "secret", time.Second, ManagementUsageQueueKey, 1)
	for range 2 {
		messages, err := client.PopUsage(ctxWithTimeout(t))
		if err != nil {
			t.Fatalf("PopUsage returned error: %v", err)
		}
		if len(messages) != 1 || messages[0] != `{"r":1}` {
			t.Fatalf("unexpected messages: %#v", messages)
		}
	}

	if redisCalls.Load() != 2 {
		t.Fatalf("expected cached redis mode to make two redis calls, got %d", redisCalls.Load())
	}
	if count := strings.Count(logs.String(), `msg="usage queue sync used redis protocol"`); count != 1 {
		t.Fatalf("expected redis mode to be logged once, got %d logs: %q", count, logs.String())
	}
}

func TestRedisQueueClientPrefersRedisBeforeHTTPFallback(t *testing.T) {
	redisServer := newRedisQueueTestServer(t, func(t *testing.T, conn net.Conn) {
		reader := bufio.NewReader(conn)
		readRESPCommand(t, reader)
		fmt.Fprint(conn, "+OK\r\n")
		readRESPCommand(t, reader)
		fmt.Fprint(conn, "*1\r\n$7\r\n{\"r\":1}\r\n")
	})
	httpCalled := false
	httpServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpCalled = true
		_, _ = w.Write([]byte(`[{"h":1}]`))
	}))
	defer httpServer.Close()

	client := NewRedisQueueClient(httpServer.URL, redisServer.URL, "secret", time.Second, ManagementUsageQueueKey, 2)
	client.httpClient.httpClient = httpServer.Client()
	messages, err := client.PopUsage(ctxWithTimeout(t))
	if err != nil {
		t.Fatalf("PopUsage returned error: %v", err)
	}
	if httpCalled {
		t.Fatal("expected redis success to skip http fallback")
	}
	if len(messages) != 1 || messages[0] != `{"r":1}` {
		t.Fatalf("unexpected messages: %#v", messages)
	}
}

func TestRedisQueueClientTreatsEmptyPopAsSuccess(t *testing.T) {
	server := newRedisQueueTestServer(t, func(t *testing.T, conn net.Conn) {
		reader := bufio.NewReader(conn)
		readRESPCommand(t, reader)
		fmt.Fprint(conn, "+OK\r\n")
		readRESPCommand(t, reader)
		fmt.Fprint(conn, "*0\r\n")
	})

	client := NewRedisQueueClient(server.URL, "", "secret", time.Second, ManagementUsageQueueKey, 1000)
	messages, err := client.PopUsage(ctxWithTimeout(t))
	if err != nil {
		t.Fatalf("PopUsage returned error: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("expected empty messages, got %#v", messages)
	}
}

func TestRedisQueueClientClassifiesAuthErrors(t *testing.T) {
	server := newRedisQueueTestServer(t, func(t *testing.T, conn net.Conn) {
		readRESPCommand(t, bufio.NewReader(conn))
		fmt.Fprint(conn, "-ERR invalid password\r\n")
	})

	client := NewRedisQueueClient(server.URL, "", "wrong", time.Second, ManagementUsageQueueKey, 1000)
	_, err := client.PopUsage(ctxWithTimeout(t))
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !errors.Is(err, ErrRedisQueueAuth) {
		t.Fatalf("expected ErrRedisQueueAuth, got %v", err)
	}
}

func TestRedisQueueClientPrefersExplicitQueueAddr(t *testing.T) {
	if got := redisQueueAddress("https://cpa.example.com", "redis-stream.example.com:6380"); got != "redis-stream.example.com:6380" {
		t.Fatalf("expected explicit redis queue addr, got %q", got)
	}
	if got := redisQueueAddress("https://cpa.example.com", "redis://redis-stream.example.com:6380"); got != "redis-stream.example.com:6380" {
		t.Fatalf("expected redis scheme to be stripped, got %q", got)
	}
	if got := redisQueueAddress("https://cpa.example.com", "http://redis-stream.example.com:6380"); got != "redis-stream.example.com:6380" {
		t.Fatalf("expected http scheme to be stripped, got %q", got)
	}
}

func TestRedisQueueClientDefaultsToManagementPortFromBaseURLHost(t *testing.T) {
	if got := redisQueueAddress("https://cpa.example.com", ""); got != "cpa.example.com:"+ManagementRedisDefaultPort {
		t.Fatalf("expected default management port from https host, got %q", got)
	}
	if got := redisQueueAddress("http://cpa.example.com", ""); got != "cpa.example.com:"+ManagementRedisDefaultPort {
		t.Fatalf("expected default management port from http host, got %q", got)
	}
	if got := redisQueueAddress("http://127.0.0.1:"+ManagementRedisDefaultPort, ""); got != "127.0.0.1:"+ManagementRedisDefaultPort {
		t.Fatalf("expected explicit port to be preserved, got %q", got)
	}
}

func TestRedisQueueClientReportsMalformedRESP(t *testing.T) {
	server := newRedisQueueTestServer(t, func(t *testing.T, conn net.Conn) {
		reader := bufio.NewReader(conn)
		readRESPCommand(t, reader)
		fmt.Fprint(conn, "+OK\r\n")
		readRESPCommand(t, reader)
		fmt.Fprint(conn, "!not-resp\r\n")
	})

	client := NewRedisQueueClient(server.URL, "", "secret", time.Second, ManagementUsageQueueKey, 1000)
	_, err := client.PopUsage(ctxWithTimeout(t))
	if err == nil || !strings.Contains(err.Error(), "read redis queue pop response") {
		t.Fatalf("expected malformed response error, got %v", err)
	}
}

type redisQueueTestServer struct {
	URL string
}

func newRedisQueueTestServer(t *testing.T, handler func(*testing.T, net.Conn)) redisQueueTestServer {
	t.Helper()
	return newRedisQueueMultiTestServer(t, 1, handler)
}

func newRedisQueueMultiTestServer(t *testing.T, connections int, handler func(*testing.T, net.Conn)) redisQueueTestServer {
	t.Helper()
	listener, err := net.Listen(cpaManagementRedisNetwork, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range connections {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			handler(t, conn)
			conn.Close()
		}
	}()
	t.Cleanup(func() { <-done })

	return redisQueueTestServer{URL: "http://" + listener.Addr().String()}
}

func readRESPCommand(t *testing.T, reader *bufio.Reader) []string {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read command header: %v", err)
	}
	var count int
	if _, err := fmt.Sscanf(line, "*%d\r\n", &count); err != nil {
		t.Fatalf("parse command header %q: %v", line, err)
	}
	parts := make([]string, 0, count)
	for range count {
		bulkHeader, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read bulk header: %v", err)
		}
		var size int
		if _, err := fmt.Sscanf(bulkHeader, "$%d\r\n", &size); err != nil {
			t.Fatalf("parse bulk header %q: %v", bulkHeader, err)
		}
		buf := make([]byte, size+2)
		if _, err := reader.Read(buf); err != nil {
			t.Fatalf("read bulk body: %v", err)
		}
		parts = append(parts, string(buf[:size]))
	}
	return parts
}

func captureRedisQueueClientInfoLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previousOutput := logrus.StandardLogger().Out
	previousFormatter := logrus.StandardLogger().Formatter
	previousLevel := logrus.GetLevel()
	logrus.SetOutput(&logs)
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	logrus.SetLevel(logrus.InfoLevel)
	t.Cleanup(func() {
		logrus.SetOutput(previousOutput)
		logrus.SetFormatter(previousFormatter)
		logrus.SetLevel(previousLevel)
	})
	return &logs
}

func ctxWithTimeout(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	return ctx
}
