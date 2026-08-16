package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProvisionLegacyUserCreatesAndReusesVerifiedAccount(t *testing.T) {
	var serverURL string
	userExists := false
	passwordActionRequired := false
	createCalls := 0
	emailCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/geoguessme/protocol/openid-connect/token":
			writeOIDCTestJSON(t, w, map[string]string{"access_token": "service-token"})
		case "/admin/realms/geoguessme/users":
			if r.Header.Get("Authorization") != "Bearer service-token" {
				t.Errorf("authorization = %q", r.Header.Get("Authorization"))
			}
			switch r.Method {
			case http.MethodGet:
				if r.URL.Query().Get("email") != "alice@example.test" || r.URL.Query().Get("exact") != "true" {
					t.Errorf("unexpected lookup query: %s", r.URL.RawQuery)
				}
				users := []keycloakUser{}
				if userExists {
					user := keycloakUser{ID: "subject-1", Email: "alice@example.test", EmailVerified: true}
					if passwordActionRequired {
						user.RequiredActions = []string{"UPDATE_PASSWORD"}
					}
					users = append(users, user)
				}
				writeOIDCTestJSON(t, w, users)
			case http.MethodPost:
				createCalls++
				var user keycloakUser
				if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
					t.Error(err)
				}
				if user.Email != "alice@example.test" || !user.EmailVerified || !containsString(user.RequiredActions, "UPDATE_PASSWORD") {
					t.Errorf("unexpected created user: %+v", user)
				}
				userExists = true
				passwordActionRequired = true
				w.Header().Set("Location", serverURL+"/admin/realms/geoguessme/users/subject-1")
				w.WriteHeader(http.StatusCreated)
			default:
				t.Errorf("unexpected users method: %s", r.Method)
			}
		case "/admin/realms/geoguessme/users/subject-1/execute-actions-email":
			emailCalls++
			if r.Method != http.MethodPut || r.URL.Query().Get("lifespan") != "86400" ||
				r.URL.Query().Get("client_id") != "geoguessme-dev" || r.URL.Query().Get("redirect_uri") != "https://geoguessme.example/login" {
				t.Errorf("unexpected action email request: %s %s", r.Method, r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL
	admin := &KeycloakAdmin{
		issuer: server.URL + "/realms/geoguessme", clientID: "geoguessme-dev",
		clientSecret: "client-secret", httpClient: server.Client(),
	}

	created, err := admin.ProvisionLegacyUser(t.Context(), " Alice@Example.test ", "https://geoguessme.example/login")
	if err != nil || !created.Created || !created.ActionEmailSent {
		t.Fatalf("first provision = %+v, %v", created, err)
	}
	passwordActionRequired = false
	reused, err := admin.ProvisionLegacyUser(t.Context(), "alice@example.test", "https://geoguessme.example/login")
	if err != nil || reused.Created || reused.ActionEmailSent {
		t.Fatalf("reused provision = %+v, %v", reused, err)
	}
	if createCalls != 1 || emailCalls != 1 {
		t.Fatalf("create calls = %d, email calls = %d", createCalls, emailCalls)
	}
}

func TestProvisionLegacyUserRejectsUnverifiedKeycloakCollision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/geoguessme/protocol/openid-connect/token":
			writeOIDCTestJSON(t, w, map[string]string{"access_token": "service-token"})
		case "/admin/realms/geoguessme/users":
			writeOIDCTestJSON(t, w, []keycloakUser{{ID: "subject-1", Email: "alice@example.test", EmailVerified: false}})
		default:
			t.Fatal("unverified collision must not trigger another Keycloak operation")
		}
	}))
	defer server.Close()
	admin := &KeycloakAdmin{
		issuer: server.URL + "/realms/geoguessme", clientID: "geoguessme-dev",
		clientSecret: "client-secret", httpClient: server.Client(),
	}
	if _, err := admin.ProvisionLegacyUser(t.Context(), "alice@example.test", "https://geoguessme.example/login"); err == nil || !strings.Contains(err.Error(), "verified email") {
		t.Fatalf("unverified collision error = %v", err)
	}
}
