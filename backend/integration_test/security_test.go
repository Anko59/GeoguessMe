package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityFeatures(t *testing.T) {
	// Check if server is running
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Skip("Server not running, skipping integration test")
		return
	}
	defer resp.Body.Close()

	t.Run("Input Validation", func(t *testing.T) {
		// Test weak password
		body := map[string]string{"username": "validuser", "password": "weak"}
		jsonBody, _ := json.Marshal(body)
		resp, err := http.Post(baseURL+"/signup", "application/json", bytes.NewBuffer(jsonBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject weak password")

		// Test invalid username
		body = map[string]string{"username": "ab", "password": "StrongPassword123"}
		jsonBody, _ = json.Marshal(body)
		resp, err = http.Post(baseURL+"/signup", "application/json", bytes.NewBuffer(jsonBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject short username")
	})

	t.Run("Rate Limiting", func(t *testing.T) {
		// Note: This test assumes the rate limit is 10 req/min
		// We'll try to hit it. If the server has already been hit by other tests,
		// this might trigger early, which is fine.

		client := &http.Client{}
		hitLimit := false

		for i := 0; i < 15; i++ {
			body := map[string]string{"username": "ratelimituser", "password": "StrongPassword123"}
			jsonBody, _ := json.Marshal(body)
			resp, err := client.Post(baseURL+"/login", "application/json", bytes.NewBuffer(jsonBody))
			require.NoError(t, err)
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusTooManyRequests {
				hitLimit = true
				break
			}
		}

		assert.True(t, hitLimit, "Should hit rate limit after multiple requests")
	})

	t.Run("File Upload Security", func(t *testing.T) {
		// Signup a user to get a token
		username := "uploadtest_" + fmt.Sprint(time.Now().UnixNano())
		token, _ := signup(t, username, "StrongPassword123")

		// Create a group
		groupID, _ := createGroup(t, token, "Upload Test Group")

		// Try to upload a "fake" image (text file renamed to .jpg)
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("photo", "fake.jpg")
		part.Write([]byte("this is just text, not an image"))
		writer.WriteField("lat", "0")
		writer.WriteField("long", "0")
		writer.WriteField("group_id", groupID)
		writer.Close()

		req, _ := http.NewRequest("POST", baseURL+"/photo/upload", body)
		req.Header.Set("Authorization", token)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		// Use unique IP for this test to avoid rate limits
		req.Header.Set("X-Forwarded-For", "10.0.0.2")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "Should reject file with invalid magic bytes")
	})

	t.Run("Authorization Enforcement", func(t *testing.T) {
		// User A creates group
		userA := "auth_userA_" + fmt.Sprint(time.Now().UnixNano())
		tokenA, _ := signup(t, userA, "StrongPassword123")
		groupID, _ := createGroup(t, tokenA, "Auth Test Group")

		// User B (not in group) tries to access leaderboard
		userB := "auth_userB_" + fmt.Sprint(time.Now().UnixNano())
		tokenB, _ := signup(t, userB, "StrongPassword123")

		req, _ := http.NewRequest("GET", baseURL+"/group/leaderboard?group_id="+groupID, nil)
		req.Header.Set("Authorization", tokenB)
		// Use unique IP for this test to avoid rate limits
		req.Header.Set("X-Forwarded-For", "10.0.0.3")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "Should forbid non-member from accessing leaderboard")
	})
}
