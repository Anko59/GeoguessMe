package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const baseURL = "http://localhost:8080"

func TestFullGameFlow(t *testing.T) {
	// Check if server is running
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Skip("Server not running, skipping integration test")
		return
	}
	defer resp.Body.Close()

	// 1. Signup User A
	userA := "userA_" + fmt.Sprint(time.Now().UnixNano())
	tokenA, _ := signup(t, userA, "TestPass123")

	// 2. Signup User B
	userB := "userB_" + fmt.Sprint(time.Now().UnixNano())
	tokenB, _ := signup(t, userB, "TestPass123")

	// 3. User A creates group
	groupID, joinCode := createGroup(t, tokenA, "Test Group")
	fmt.Printf("Group Created: %s, Code: %s\n", groupID, joinCode)

	// 4. User B joins group
	joinGroup(t, tokenB, joinCode)

	// 5. User A uploads photo
	photoID := uploadPhoto(t, tokenA, groupID)
	fmt.Printf("Photo Uploaded: %s\n", photoID)

	// 6. User B guesses
	// We need to wait a bit for async processing if any (but here it's sync)
	submitGuess(t, tokenB, photoID, 51.505, -0.09)

	// 7. Verify Leaderboard
	verifyLeaderboard(t, tokenA, groupID, userB)
}

func signup(t *testing.T, username, password string) (string, string) {
	body := map[string]string{"username": username, "password": password}
	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", baseURL+"/signup", bytes.NewBuffer(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// Use random IP to avoid rate limiting collisions between tests
	req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.%d.%d", time.Now().UnixNano()%255, time.Now().UnixNano()%255))

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)
	return res["token"].(string), res["user"].(map[string]interface{})["id"].(string)
}

func createGroup(t *testing.T, token, name string) (string, string) {
	body := map[string]string{"name": name}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/group/create", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	fmt.Printf("Create Group Response: %s\n", string(bodyBytes))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var res map[string]interface{}
	json.Unmarshal(bodyBytes, &res)
	return res["id"].(string), res["code"].(string)
}

func joinGroup(t *testing.T, token, code string) {
	body := map[string]string{"code": code}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/group/join", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func uploadPhoto(t *testing.T, token, groupID string) string {
	// Create a dummy image
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("photo", "test.jpg")
	// Use valid JPEG magic bytes
	part.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01})
	part.Write([]byte("fake image content"))
	writer.WriteField("lat", "51.505")
	writer.WriteField("long", "-0.09")
	writer.WriteField("group_id", groupID)
	writer.Close()

	req, _ := http.NewRequest("POST", baseURL+"/photo/upload", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var res map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)
	return res["id"].(string)
}

func submitGuess(t *testing.T, token, photoID string, lat, lng float64) {
	body := map[string]interface{}{
		"photo_id": photoID,
		"lat":      lat,
		"lng":      lng,
	}
	jsonBody, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", baseURL+"/guess", bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func verifyLeaderboard(t *testing.T, token, groupID, expectedUser string) {
	req, _ := http.NewRequest("GET", baseURL+"/group/leaderboard?group_id="+groupID, nil)
	req.Header.Set("Authorization", token)

	client := &http.Client{}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var res []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&res)

	found := false
	for _, entry := range res {
		if entry["username"] == expectedUser {
			found = true
			require.GreaterOrEqual(t, entry["score"].(float64), 0.0)
		}
	}
	assert.True(t, found, "User B should be in leaderboard")
}
