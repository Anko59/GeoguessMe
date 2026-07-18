// reconnect-rehearsal executes a deterministic reconnect/load rehearsal against
// a GeoGuessMe test stack. It exercises concurrent clients, WebSocket
// disconnect / renewed-ticket / cursor catch-up / live delivery, and exact-once
// message behavior, capturing bounded latency, error, and throughput evidence
// without sleeps or retry masking.
//
// Usage:
//
//	go run . -base-url http://host.docker.internal:18080
//
// The tool exits 0 when every assertion passes and non-zero on any failure.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// ------- command-line flags -------

var baseURL = flag.String("base-url", "http://localhost:8080", "GeoGuessMe public base URL")

// ------- types -------

type credentials struct {
	Username string
	Password string
	Access   string
	Refresh  string
}

type groupRef struct {
	ID   string
	Code string
}

type wsMessage struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	UserID    string `json:"user_id"`
	Username  string `json:"username"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type cursorPage struct {
	Items      []wsMessage `json:"items"`
	NextCursor string      `json:"next_cursor"`
}

type ticketResponse struct {
	Ticket string `json:"ticket"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
}

type result struct {
	Sent     int32
	Received int32
	Errors   int32
	// Latencies in microseconds per received message delivery end-to-end
	// (send call -> ReadMessage return on a different socket).
	Latencies []int64
	mu        sync.Mutex
}

func (r *result) addLatency(us int64) {
	r.mu.Lock()
	r.Latencies = append(r.Latencies, us)
	r.mu.Unlock()
}

func (r *result) addError() { atomic.AddInt32(&r.Errors, 1) }
func (r *result) addSent()  { atomic.AddInt32(&r.Sent, 1) }
func (r *result) incRecv()  { atomic.AddInt32(&r.Received, 1) }

// ------- helpers -------

func doJSON(method, path string, body any, access string, cookies []*http.Cookie) (*http.Response, []byte, error) {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
	}
	// Separate path and query so url.JoinPath does not encode the '?'.
	parts := strings.SplitN(path, "?", 2)
	basePath := parts[0]
	query := ""
	if len(parts) == 2 {
		query = parts[1]
	}
	joined, err := url.JoinPath(*baseURL, basePath)
	if err != nil {
		return nil, nil, err
	}
	if query != "" {
		joined += "?" + query
	}
	req, err := http.NewRequest(method, joined, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	return resp, buf.Bytes(), nil
}

func wsBase() string {
	u, err := url.Parse(*baseURL)
	if err != nil {
		return "ws://localhost:8080"
	}
	return "ws://" + u.Host
}

func dialWS(groupID, ticket string) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set("Origin", *baseURL)
	conn, resp, err := websocket.DefaultDialer.Dial(
		wsBase()+"/api/v1/ws?group_id="+url.QueryEscape(groupID)+"&ticket="+url.QueryEscape(ticket),
		header,
	)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return conn, err
}

func mustVal[T any](t T, err error) T {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %v\n", err)
		os.Exit(1)
	}
	return t
}

func mustOK(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %v\n", err)
		os.Exit(1)
	}
}
