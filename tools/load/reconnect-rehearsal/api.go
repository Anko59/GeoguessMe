package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ------- API helpers -------

func signup(username, email, password string) (credentials, error) {
	resp, body, err := doJSON(http.MethodPost, "/api/v1/auth/signup",
		map[string]string{"username": username, "email": email, "password": password},
		"", nil)
	if err != nil {
		return credentials{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return credentials{}, fmt.Errorf("signup returned %d: %s", resp.StatusCode, string(body))
	}
	var lr loginResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return credentials{}, err
	}
	c := credentials{Username: username, Password: password, Access: lr.AccessToken}
	for _, ck := range resp.Cookies() {
		if ck.Name == "refresh_token" {
			c.Refresh = ck.Value
		}
	}
	return c, nil
}

func createGroup(access, name string) (groupRef, error) {
	resp, body, err := doJSON(http.MethodPost, "/api/v1/group/create",
		map[string]string{"name": name}, access, nil)
	if err != nil {
		return groupRef{}, err
	}
	if resp.StatusCode != http.StatusCreated {
		return groupRef{}, fmt.Errorf("create group returned %d: %s", resp.StatusCode, string(body))
	}
	var g struct {
		ID   string `json:"id"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &g); err != nil {
		return groupRef{}, err
	}
	return groupRef{ID: g.ID, Code: g.Code}, nil
}

func joinGroup(access, code string) error {
	resp, body, err := doJSON(http.MethodPost, "/api/v1/group/join",
		map[string]string{"code": code}, access, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("join group returned %d: %s", resp.StatusCode, string(body))
	}
	_ = body
	return nil
}

func getWSTicket(access, groupID string) (string, error) {
	resp, body, err := doJSON(http.MethodPost, "/api/v1/ws/ticket?group_id="+url.QueryEscape(groupID),
		nil, access, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("ws ticket returned %d: %s", resp.StatusCode, string(body))
	}
	var tr ticketResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	return tr.Ticket, nil
}

func refreshAuth(c credentials) (credentials, error) {
	if c.Refresh == "" {
		// Login fresh.
		resp, body, err := doJSON(http.MethodPost, "/api/v1/auth/login",
			map[string]string{"username": c.Username, "password": c.Password},
			"", nil)
		if err != nil {
			return c, err
		}
		if resp.StatusCode != http.StatusOK {
			return c, fmt.Errorf("login returned %d: %s", resp.StatusCode, string(body))
		}
		var lr loginResponse
		if err := json.Unmarshal(body, &lr); err != nil {
			return c, err
		}
		c.Access = lr.AccessToken
		for _, ck := range resp.Cookies() {
			if ck.Name == "refresh_token" {
				c.Refresh = ck.Value
			}
		}
		return c, nil
	}
	resp, body, err := doJSON(http.MethodPost, "/api/v1/auth/refresh", nil, "",
		[]*http.Cookie{{Name: "refresh_token", Value: c.Refresh}})
	if err != nil {
		// Fallback to login.
		return refreshAuth(credentials{Username: c.Username, Password: c.Password})
	}
	if resp.StatusCode != http.StatusOK {
		return refreshAuth(credentials{Username: c.Username, Password: c.Password})
	}
	var lr loginResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return c, fmt.Errorf("refresh parse: %w", err)
	}
	c.Access = lr.AccessToken
	for _, ck := range resp.Cookies() {
		if ck.Name == "refresh_token" {
			c.Refresh = ck.Value
		}
	}
	return c, nil
}

func getMessages(access, groupID, cursor string) (cursorPage, error) {
	path := "/api/v1/group/messages?group_id=" + url.QueryEscape(groupID)
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	resp, body, err := doJSON(http.MethodGet, path, nil, access, nil)
	if err != nil {
		return cursorPage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return cursorPage{}, fmt.Errorf("messages returned %d: %s", resp.StatusCode, string(body))
	}
	var page cursorPage
	if err := json.Unmarshal(body, &page); err != nil {
		return cursorPage{}, err
	}
	return page, nil
}

// getLatestCursor returns the cursor of the most recent message, or empty
// if no messages exist.
func getLatestCursor(access, groupID string) (string, error) {
	page, err := getMessages(access, groupID, "")
	if err != nil {
		return "", err
	}
	if len(page.Items) > 0 {
		last := page.Items[len(page.Items)-1]
		createdAt, err := time.Parse(time.RFC3339Nano, last.CreatedAt)
		if err != nil {
			return "", err
		}
		return encodeCursor(createdAt, last.ID), nil
	}
	return "", nil
}

func encodeCursor(createdAt time.Time, id string) string {
	payload := strconv.FormatInt(createdAt.UnixNano(), 10) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func randomSuffix() string {
	// Keep under ~10 chars so username + suffix stays within 30-char limit.
	return strings.ToLower(strconv.FormatInt(time.Now().UnixMilli()%1000000, 36))
}

func fail(reason string, res *result) {
	fmt.Fprintf(os.Stderr, "\nREHEARSAL FAILED: %s\n", reason)
	fmt.Fprintf(os.Stderr, "errors: %d sent: %d received: %d\n",
		atomic.LoadInt32(&res.Errors), atomic.LoadInt32(&res.Sent), atomic.LoadInt32(&res.Received))
	os.Exit(1)
}
