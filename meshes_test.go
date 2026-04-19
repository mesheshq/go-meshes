package meshes

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/deepmap/oapi-codegen/pkg/types"
	"github.com/google/uuid"
)

func TestParseUpdateConnectionConflictResponse(t *testing.T) {
	resp, err := ParseUpdateConnectionResponse(
		newJSONHTTPResponse(http.StatusConflict, `{"message":"connection already exists"}`),
	)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if resp.StatusCode() != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, resp.StatusCode())
	}
	if got := responseMessageField(t, resp, "JSON409"); got != "connection already exists" {
		t.Errorf("expected conflict message, got %s", got)
	}
}

// --- Helpers ---

var testCreds = MeshesCredentials{
	AccessKey: "mk_test_access_key",
	SecretKey: "mk_test_secret_key_value",
	OrgID:     "00000000-0000-0000-0000-000000000099",
}

func mustUUID(s string) openapi_types.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func jsonResponse(t *testing.T, w http.ResponseWriter, status int, body interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("failed to encode response: %v", err)
	}
}

func newJSONHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func responseMessageField(t *testing.T, resp interface{}, field string) string {
	t.Helper()

	value := reflect.ValueOf(resp)
	if value.Kind() != reflect.Ptr || value.IsNil() {
		t.Fatalf("expected pointer response, got %T", resp)
	}

	fieldValue := value.Elem().FieldByName(field)
	if !fieldValue.IsValid() {
		t.Fatalf("expected field %s on %T", field, resp)
	}
	if fieldValue.IsNil() {
		t.Fatalf("expected field %s to be set", field)
	}

	message := fieldValue.Elem().FieldByName("Message")
	if !message.IsValid() {
		t.Fatalf("expected Message field on %s", field)
	}

	return message.String()
}

func newTestManagementClient(t *testing.T, server *httptest.Server) *ClientWithResponses {
	t.Helper()
	client, err := NewManagementClient(testCreds, WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

func newTestEventClient(t *testing.T, server *httptest.Server) *ClientWithResponses {
	t.Helper()
	client, err := NewEventClient("pub_test_key", WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return client
}

// --- JWT Tests ---

func TestMintJWT(t *testing.T) {
	token, err := mintJWT(testCreds)
	if err != nil {
		t.Fatalf("mintJWT failed: %v", err)
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	// Decode and verify header
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("failed to decode header: %v", err)
	}
	var header map[string]string
	json.Unmarshal(headerJSON, &header)

	if header["alg"] != "HS256" {
		t.Errorf("expected alg HS256, got %s", header["alg"])
	}
	if header["typ"] != "JWT" {
		t.Errorf("expected typ JWT, got %s", header["typ"])
	}
	if header["kid"] != testCreds.AccessKey {
		t.Errorf("expected kid %s, got %s", testCreds.AccessKey, header["kid"])
	}

	// Decode and verify payload
	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}
	var payload map[string]interface{}
	json.Unmarshal(payloadJSON, &payload)

	if payload["iss"] != "urn:meshes:m2m:"+testCreds.AccessKey {
		t.Errorf("unexpected iss: %v", payload["iss"])
	}
	if payload["aud"] != "meshes-api" {
		t.Errorf("unexpected aud: %v", payload["aud"])
	}
	if payload["org"] != testCreds.OrgID {
		t.Errorf("unexpected org: %v", payload["org"])
	}

	// Verify exp is ~30s from iat
	iat := int64(payload["iat"].(float64))
	exp := int64(payload["exp"].(float64))
	if exp-iat != 30 {
		t.Errorf("expected 30s expiry, got %ds", exp-iat)
	}

	// Verify signature
	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(testCreds.SecretKey))
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != expectedSig {
		t.Error("JWT signature verification failed")
	}
}

func TestManagementClientSendsJWT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("expected Bearer auth, got %s", auth)
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			t.Errorf("expected valid JWT, got %s", token)
		}

		// Verify the kid matches our access key
		headerJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
		var header map[string]string
		json.Unmarshal(headerJSON, &header)
		if header["kid"] != testCreds.AccessKey {
			t.Errorf("expected kid %s, got %s", testCreds.AccessKey, header["kid"])
		}

		jsonResponse(t, w, 200, map[string]interface{}{
			"count": 0, "limit": 20, "next_cursor": nil, "records": []interface{}{},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.GetWorkspacesWithResponse(context.Background())
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode())
	}
}

func TestEventClientSendsPublishableKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Meshes-Publishable-Key")
		if key != "pub_test_key" {
			t.Errorf("expected pub_test_key, got %s", key)
		}
		jsonResponse(t, w, 201, map[string]interface{}{
			"event": map[string]interface{}{
				"id":         "00000000-0000-0000-0000-000000000001",
				"event":      "user.signup",
				"workspace":  "00000000-0000-0000-0000-000000000002",
				"created_at": time.Now().Format(time.RFC3339),
				"created_by": "test",
			},
		})
	}))
	defer server.Close()

	client := newTestEventClient(t, server)
	resp, err := client.CreateEventWithResponse(context.Background(), CreateEventJSONRequestBody{
		Event: "user.signup",
		Payload: EventPayload{
			Email: ptrTo(openapi_types.Email("test@example.com")),
		},
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode())
	}
}

// --- Credential Validation ---

func TestManagementClientRequiresAllCredentials(t *testing.T) {
	tests := []struct {
		name  string
		creds MeshesCredentials
	}{
		{"missing access key", MeshesCredentials{SecretKey: "s", OrgID: "o"}},
		{"missing secret key", MeshesCredentials{AccessKey: "a", OrgID: "o"}},
		{"missing org id", MeshesCredentials{AccessKey: "a", SecretKey: "s"}},
		{"all empty", MeshesCredentials{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewManagementClient(tt.creds)
			if err == nil {
				t.Error("expected error for missing credentials")
			}
		})
	}
}

func TestEventClientRequiresPublishableKey(t *testing.T) {
	_, err := NewEventClient("")
	if err == nil {
		t.Error("expected error for empty publishable key")
	}
}

// --- Default URL + Override ---

func TestDefaultServerURL(t *testing.T) {
	// Can't actually hit the real server, but verify the client constructs
	client, err := NewManagementClient(testCreds)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestWithServerURLOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(t, w, 200, map[string]interface{}{
			"count": 0, "limit": 20, "next_cursor": nil, "records": []interface{}{},
		})
	}))
	defer server.Close()

	client, err := NewManagementClient(testCreds, WithServerURL(server.URL))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := client.GetWorkspacesWithResponse(context.Background())
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode())
	}
}

// --- API Tests ---

func TestGetWorkspaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workspaces" {
			t.Errorf("expected /api/v1/workspaces, got %s", r.URL.Path)
		}
		jsonResponse(t, w, 200, map[string]interface{}{
			"count": 1, "limit": 20, "next_cursor": nil,
			"records": []map[string]interface{}{
				{
					"id":         "00000000-0000-0000-0000-000000000001",
					"name":       "My Workspace",
					"created_at": "2024-01-01T00:00:00Z",
					"updated_at": "2024-01-01T00:00:00Z",
				},
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.GetWorkspacesWithResponse(context.Background())
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if resp.JSON200.Count != 1 {
		t.Errorf("expected count 1, got %d", resp.JSON200.Count)
	}
	if len(resp.JSON200.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.JSON200.Records))
	}
	if resp.JSON200.Records[0].Name != "My Workspace" {
		t.Errorf("expected 'My Workspace', got %s", resp.JSON200.Records[0].Name)
	}
}

func TestCreateWorkspace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if req["name"] != "New Workspace" {
			t.Errorf("expected name 'New Workspace', got %v", req["name"])
		}

		jsonResponse(t, w, 201, map[string]interface{}{
			"workspace": map[string]interface{}{
				"id":         "00000000-0000-0000-0000-000000000099",
				"name":       "New Workspace",
				"created_at": "2024-01-01T00:00:00Z",
				"updated_at": "2024-01-01T00:00:00Z",
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.CreateWorkspaceWithResponse(context.Background(), CreateWorkspaceJSONRequestBody{
		Name: "New Workspace",
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != 201 {
		t.Fatalf("expected 201, got %d", resp.StatusCode())
	}
	if resp.JSON201.Workspace.Name != "New Workspace" {
		t.Errorf("expected 'New Workspace', got %s", resp.JSON201.Workspace.Name)
	}
}

func TestGetConnections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(t, w, 200, map[string]interface{}{
			"count": 1, "limit": 20, "next_cursor": nil,
			"records": []map[string]interface{}{
				{
					"id":         "00000000-0000-0000-0000-000000000010",
					"workspace":  "00000000-0000-0000-0000-000000000001",
					"type":       "hubspot",
					"name":       "My HubSpot",
					"metadata":   map[string]interface{}{"portal_id": "12345"},
					"created_at": "2024-01-01T00:00:00Z",
					"updated_at": "2024-01-01T00:00:00Z",
					"created_by": "user_123",
				},
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.GetConnectionsWithResponse(context.Background())
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}

	conn := resp.JSON200.Records[0]
	if conn.Type != ConnectionTypeHubspot {
		t.Errorf("expected hubspot, got %s", conn.Type)
	}
	if conn.Name != "My HubSpot" {
		t.Errorf("expected 'My HubSpot', got %s", conn.Name)
	}
}

func TestCreateConnectionSupportsNewIntegrationTypes(t *testing.T) {
	workspaceID := mustUUID("00000000-0000-0000-0000-000000000001")

	tests := []struct {
		name         string
		requestType  CreateConnectionJSONBodyType
		responseType ConnectionType
		expectedType string
	}{
		{
			name:         "customer io",
			requestType:  CreateConnectionJSONBodyTypeCustomerIo,
			responseType: ConnectionTypeCustomerIo,
			expectedType: "customer_io",
		},
		{
			name:         "discord",
			requestType:  CreateConnectionJSONBodyTypeDiscord,
			responseType: ConnectionTypeDiscord,
			expectedType: "discord",
		},
		{
			name:         "sendgrid",
			requestType:  CreateConnectionJSONBodyTypeSendgrid,
			responseType: ConnectionTypeSendgrid,
			expectedType: "sendgrid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/api/v1/connections" {
					t.Errorf("expected /api/v1/connections, got %s", r.URL.Path)
				}

				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}

				var req map[string]interface{}
				if err := json.Unmarshal(body, &req); err != nil {
					t.Fatalf("failed to decode request: %v", err)
				}
				if req["type"] != tt.expectedType {
					t.Errorf("expected type %s, got %v", tt.expectedType, req["type"])
				}

				jsonResponse(t, w, http.StatusCreated, map[string]interface{}{
					"connection": map[string]interface{}{
						"active":          false,
						"created_at":      "2024-01-01T00:00:00Z",
						"created_by":      "user_123",
						"id":              "00000000-0000-0000-0000-000000000010",
						"inactive_reason": "manually_inactivated",
						"metadata":        map[string]interface{}{"source": "test"},
						"name":            "Test " + tt.name,
						"type":            tt.expectedType,
						"updated_at":      "2024-01-01T00:00:00Z",
						"workspace":       workspaceID.String(),
					},
				})
			}))
			defer server.Close()

			client := newTestManagementClient(t, server)
			resp, err := client.CreateConnectionWithResponse(context.Background(), CreateConnectionJSONRequestBody{
				Metadata:  map[string]interface{}{"source": "test"},
				Name:      "Test " + tt.name,
				Type:      tt.requestType,
				Workspace: workspaceID,
			})
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.JSON201 == nil {
				t.Fatal("expected JSON201 to be non-nil")
			}

			conn := resp.JSON201.Connection
			if conn.Type != tt.responseType {
				t.Errorf("expected type %s, got %s", tt.responseType, conn.Type)
			}
			if conn.Active == nil || *conn.Active {
				t.Fatalf("expected active=false, got %v", conn.Active)
			}
			if conn.InactiveReason == nil || *conn.InactiveReason != ManuallyInactivated {
				t.Fatalf("expected inactive reason %s, got %v", ManuallyInactivated, conn.InactiveReason)
			}
		})
	}
}

func TestCreateEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		json.Unmarshal(body, &req)

		if req["event"] != "user.signup" {
			t.Errorf("expected event 'user.signup', got %v", req["event"])
		}

		payload := req["payload"].(map[string]interface{})
		if payload["email"] != "jane@example.com" {
			t.Errorf("expected email 'jane@example.com', got %v", payload["email"])
		}
		if payload["plan"] != "pro" {
			t.Errorf("expected plan 'pro', got %v", payload["plan"])
		}

		jsonResponse(t, w, 201, map[string]interface{}{
			"event": map[string]interface{}{
				"id":         "00000000-0000-0000-0000-000000000050",
				"event":      "user.signup",
				"workspace":  "00000000-0000-0000-0000-000000000001",
				"created_at": "2024-01-15T10:30:00Z",
				"created_by": "pub_key",
			},
		})
	}))
	defer server.Close()

	client := newTestEventClient(t, server)

	payload := EventPayload{
		Email:     ptrTo(openapi_types.Email("jane@example.com")),
		FirstName: ptrTo("Jane"),
		LastName:  ptrTo("Doe"),
	}
	payload.Set("plan", "pro")

	resp, err := client.CreateEventWithResponse(context.Background(), CreateEventJSONRequestBody{
		Event:   "user.signup",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode())
	}
	if resp.JSON201.Event.Event != "user.signup" {
		t.Errorf("expected 'user.signup', got %s", resp.JSON201.Event.Event)
	}
}

func TestGetRulesWithQueryParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("event") != "user.signup" {
			t.Errorf("expected event query param 'user.signup', got %s", r.URL.Query().Get("event"))
		}

		jsonResponse(t, w, 200, map[string]interface{}{
			"count": 1, "limit": 20, "next_cursor": nil,
			"records": []map[string]interface{}{
				{
					"id":         "00000000-0000-0000-0000-000000000020",
					"workspace":  "00000000-0000-0000-0000-000000000001",
					"connection": "00000000-0000-0000-0000-000000000010",
					"event":      "user.signup",
					"type":       "integration",
					"metadata":   map[string]interface{}{"action": "add_contact"},
					"active":     true,
					"created_at": "2024-01-01T00:00:00Z",
					"updated_at": "2024-01-01T00:00:00Z",
					"created_by": "user_123",
				},
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	event := "user.signup"
	resp, err := client.GetRulesWithResponse(context.Background(), &GetRulesParams{
		Event: &event,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200.Records[0].Metadata.Action != "add_contact" {
		t.Errorf("expected action 'add_contact', got %s", resp.JSON200.Records[0].Metadata.Action)
	}
}

func TestListSessionsWithQueryParams(t *testing.T) {
	workspaceID := mustUUID("00000000-0000-0000-0000-0000000000aa")
	limit := 10
	cursor := "cursor_123"
	status := Active
	resource := "contacts"
	resourceID := "contact_123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sessions" {
			t.Errorf("expected /api/v1/sessions, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("workspace_id"); got != workspaceID.String() {
			t.Errorf("expected workspace_id %s, got %s", workspaceID, got)
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("expected limit 10, got %s", got)
		}
		if got := r.URL.Query().Get("cursor"); got != cursor {
			t.Errorf("expected cursor %s, got %s", cursor, got)
		}
		if got := r.URL.Query().Get("status"); got != string(status) {
			t.Errorf("expected status %s, got %s", status, got)
		}
		if got := r.URL.Query().Get("resource"); got != resource {
			t.Errorf("expected resource %s, got %s", resource, got)
		}
		if got := r.URL.Query().Get("resource_id"); got != resourceID {
			t.Errorf("expected resource_id %s, got %s", resourceID, got)
		}

		jsonResponse(t, w, 200, map[string]interface{}{
			"count":       1,
			"limit":       10,
			"next_cursor": nil,
			"records": []map[string]interface{}{
				{
					"created_at":       "2024-02-01T10:00:00Z",
					"expires_at":       "2024-02-01T10:30:00Z",
					"external_user_id": "user_ext_123",
					"is_expired":       false,
					"resource":         resource,
					"resource_id":      resourceID,
					"role":             "member",
					"session_id":       "sess_123",
					"session_type":     "resource",
					"status":           "active",
					"workspace_id":     workspaceID.String(),
				},
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.ListSessionsWithResponse(context.Background(), &ListSessionsParams{
		Limit:       &limit,
		Cursor:      &cursor,
		WorkspaceId: workspaceID,
		Status:      &status,
		Resource:    &resource,
		ResourceId:  &resourceID,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if resp.JSON200.Count != 1 {
		t.Fatalf("expected count 1, got %d", resp.JSON200.Count)
	}
	record := resp.JSON200.Records[0]
	if record.SessionId != "sess_123" {
		t.Errorf("expected session ID sess_123, got %s", record.SessionId)
	}
	if record.Role != "member" {
		t.Errorf("expected role member, got %s", record.Role)
	}
	if record.SessionType != "resource" {
		t.Errorf("expected session_type resource, got %s", record.SessionType)
	}
	if record.ExternalUserId == nil || *record.ExternalUserId != "user_ext_123" {
		t.Fatalf("expected external_user_id user_ext_123, got %v", record.ExternalUserId)
	}
	if record.Resource == nil || *record.Resource != resource {
		t.Fatalf("expected resource %s, got %v", resource, record.Resource)
	}
	if record.ResourceId == nil || *record.ResourceId != resourceID {
		t.Fatalf("expected resource_id %s, got %v", resourceID, record.ResourceId)
	}
}

func TestCreateSession(t *testing.T) {
	workspaceID := mustUUID("00000000-0000-0000-0000-0000000000ab")
	externalUserID := "user_ext_456"
	launchPage := CreateSessionJSONBodyLaunchPageEvents
	launchTTLSeconds := 300
	resource := "contacts"
	resourceID := "contact_456"
	role := Member
	scopes := []CreateSessionJSONBodyScopes{EventsPayloadRead}
	sessionType := CreateSessionJSONBodySessionTypeResource
	ttlSeconds := 1800

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sessions" {
			t.Errorf("expected /api/v1/sessions, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected application/json content type, got %s", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}

		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req["workspace_id"] != workspaceID.String() {
			t.Errorf("expected workspace_id %s, got %v", workspaceID, req["workspace_id"])
		}
		if req["external_user_id"] != externalUserID {
			t.Errorf("expected external_user_id %s, got %v", externalUserID, req["external_user_id"])
		}
		if req["launch_page"] != string(launchPage) {
			t.Errorf("expected launch_page %s, got %v", launchPage, req["launch_page"])
		}
		if req["resource"] != resource {
			t.Errorf("expected resource %s, got %v", resource, req["resource"])
		}
		if req["resource_id"] != resourceID {
			t.Errorf("expected resource_id %s, got %v", resourceID, req["resource_id"])
		}
		if req["role"] != string(role) {
			t.Errorf("expected role %s, got %v", role, req["role"])
		}
		if req["session_type"] != string(sessionType) {
			t.Errorf("expected session_type %s, got %v", sessionType, req["session_type"])
		}

		allowedOrigins, ok := req["allowed_origins"].([]interface{})
		if !ok || len(allowedOrigins) != 2 {
			t.Fatalf("expected two allowed origins, got %v", req["allowed_origins"])
		}
		if allowedOrigins[0] != "https://app.example.com" || allowedOrigins[1] != "https://admin.example.com" {
			t.Errorf("unexpected allowed_origins: %v", allowedOrigins)
		}

		scopeValues, ok := req["scopes"].([]interface{})
		if !ok || len(scopeValues) != 1 || scopeValues[0] != string(EventsPayloadRead) {
			t.Fatalf("expected scopes [%s], got %v", EventsPayloadRead, req["scopes"])
		}

		if req["launch_ttl_seconds"] != float64(launchTTLSeconds) {
			t.Errorf("expected launch_ttl_seconds %d, got %v", launchTTLSeconds, req["launch_ttl_seconds"])
		}
		if req["ttl_seconds"] != float64(ttlSeconds) {
			t.Errorf("expected ttl_seconds %d, got %v", ttlSeconds, req["ttl_seconds"])
		}

		jsonResponse(t, w, 200, map[string]interface{}{
			"access_token":      "access_123",
			"expires_at":        "2024-02-01T10:30:00Z",
			"expires_in":        1800,
			"launch_expires_at": "2024-02-01T10:10:00Z",
			"launch_token":      "launch_123",
			"launch_url":        "https://app.meshes.io/launch/launch_123",
			"resource":          resource,
			"resource_id":       resourceID,
			"role":              "member",
			"session_id":        "sess_456",
			"session_type":      "resource",
			"workspace_id":      workspaceID.String(),
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.CreateSessionWithResponse(context.Background(), CreateSessionJSONRequestBody{
		AllowedOrigins:   &[]string{"https://app.example.com", "https://admin.example.com"},
		ExternalUserId:   &externalUserID,
		LaunchPage:       &launchPage,
		LaunchTtlSeconds: &launchTTLSeconds,
		Resource:         &resource,
		ResourceId:       &resourceID,
		Role:             &role,
		Scopes:           &scopes,
		SessionType:      &sessionType,
		TtlSeconds:       &ttlSeconds,
		WorkspaceId:      workspaceID,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if resp.JSON200.SessionId != "sess_456" {
		t.Errorf("expected session ID sess_456, got %s", resp.JSON200.SessionId)
	}
	if resp.JSON200.Role != "member" {
		t.Errorf("expected role member, got %s", resp.JSON200.Role)
	}
	if resp.JSON200.SessionType != "resource" {
		t.Errorf("expected session_type resource, got %s", resp.JSON200.SessionType)
	}
	if resp.JSON200.Resource == nil || *resp.JSON200.Resource != resource {
		t.Fatalf("expected resource %s, got %v", resource, resp.JSON200.Resource)
	}
	if resp.JSON200.ResourceId == nil || *resp.JSON200.ResourceId != resourceID {
		t.Fatalf("expected resource_id %s, got %v", resourceID, resp.JSON200.ResourceId)
	}
	if resp.JSON200.WorkspaceId != workspaceID.String() {
		t.Errorf("expected workspace ID %s, got %s", workspaceID, resp.JSON200.WorkspaceId)
	}
}

func TestRevokeSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sessions/sess_revoke" {
			t.Errorf("expected revoke path, got %s", r.URL.Path)
		}

		jsonResponse(t, w, 200, map[string]interface{}{
			"revoked": true,
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.RevokeSessionWithResponse(context.Background(), "sess_revoke")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if !resp.JSON200.Revoked {
		t.Error("expected revoked=true")
	}
}

func TestRefreshSession(t *testing.T) {
	resource := "contacts"
	resourceID := "contact_789"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sessions/sess_refresh/refresh" {
			t.Errorf("expected refresh path, got %s", r.URL.Path)
		}

		jsonResponse(t, w, 200, map[string]interface{}{
			"access_token": "access_refresh_123",
			"expires_at":   "2024-02-01T11:00:00Z",
			"expires_in":   1200,
			"resource":     resource,
			"resource_id":  resourceID,
			"role":         "admin",
			"session_id":   "sess_refresh",
			"session_type": "resource",
			"workspace_id": "00000000-0000-0000-0000-0000000000ac",
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.RefreshSessionWithResponse(context.Background(), "sess_refresh")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if resp.JSON200.AccessToken != "access_refresh_123" {
		t.Errorf("expected access token access_refresh_123, got %s", resp.JSON200.AccessToken)
	}
	if resp.JSON200.Role != "admin" {
		t.Errorf("expected role admin, got %s", resp.JSON200.Role)
	}
	if resp.JSON200.SessionId != "sess_refresh" {
		t.Errorf("expected session ID sess_refresh, got %s", resp.JSON200.SessionId)
	}
	if resp.JSON200.SessionType != "resource" {
		t.Errorf("expected session_type resource, got %s", resp.JSON200.SessionType)
	}
	if resp.JSON200.Resource == nil || *resp.JSON200.Resource != resource {
		t.Fatalf("expected resource %s, got %v", resource, resp.JSON200.Resource)
	}
	if resp.JSON200.ResourceId == nil || *resp.JSON200.ResourceId != resourceID {
		t.Fatalf("expected resource_id %s, got %v", resourceID, resp.JSON200.ResourceId)
	}
}

func TestCreateSessionBadRequest(t *testing.T) {
	workspaceID := mustUUID("00000000-0000-0000-0000-0000000000ab")
	badSessionType := CreateSessionJSONBodySessionType("invalid")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(t, w, http.StatusBadRequest, map[string]interface{}{
			"message": "invalid session_type",
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.CreateSessionWithResponse(context.Background(), CreateSessionJSONRequestBody{
		SessionType: &badSessionType,
		WorkspaceId: workspaceID,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON400 == nil {
		t.Fatal("expected JSON400 to be non-nil")
	}
	if resp.JSON400.Message != "invalid session_type" {
		t.Errorf("expected invalid session_type message, got %s", resp.JSON400.Message)
	}
}

func TestGetWorkspaceRulesWithQueryParams(t *testing.T) {
	workspaceID := mustUUID("00000000-0000-0000-0000-0000000000ae")
	event := "user.signup"
	resource := "contacts"
	resourceID := "contact_123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workspaces/"+workspaceID.String()+"/rules" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("event"); got != event {
			t.Errorf("expected event %s, got %s", event, got)
		}
		if got := r.URL.Query().Get("resource"); got != resource {
			t.Errorf("expected resource %s, got %s", resource, got)
		}
		if got := r.URL.Query().Get("resource_id"); got != resourceID {
			t.Errorf("expected resource_id %s, got %s", resourceID, got)
		}

		jsonResponse(t, w, 200, map[string]interface{}{
			"count":       1,
			"limit":       20,
			"next_cursor": nil,
			"records": []map[string]interface{}{
				{
					"id":          "00000000-0000-0000-0000-000000000021",
					"workspace":   workspaceID.String(),
					"connection":  "00000000-0000-0000-0000-000000000010",
					"event":       event,
					"resource":    resource,
					"resource_id": resourceID,
					"type":        "integration",
					"metadata":    map[string]interface{}{"action": "sync_contact"},
					"active":      true,
					"created_at":  "2024-01-01T00:00:00Z",
					"updated_at":  "2024-01-01T00:00:00Z",
					"created_by":  "user_123",
				},
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.GetWorkspaceRulesWithResponse(context.Background(), workspaceID, &GetWorkspaceRulesParams{
		Event:      &event,
		Resource:   &resource,
		ResourceId: &resourceID,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if resp.JSON200.Count != 1 {
		t.Fatalf("expected count 1, got %d", resp.JSON200.Count)
	}
	record := resp.JSON200.Records[0]
	if record.Metadata.Action != "sync_contact" {
		t.Errorf("expected action sync_contact, got %s", record.Metadata.Action)
	}
	if record.Resource == nil || *record.Resource != resource {
		t.Fatalf("expected resource %s, got %v", resource, record.Resource)
	}
	if record.ResourceId == nil || *record.ResourceId != resourceID {
		t.Fatalf("expected resource_id %s, got %v", resourceID, record.ResourceId)
	}
}

func TestGetWorkspaceEventTypes(t *testing.T) {
	workspaceID := mustUUID("00000000-0000-0000-0000-0000000000ad")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workspaces/"+workspaceID.String()+"/event-types" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}

		jsonResponse(t, w, 200, []map[string]interface{}{
			{
				"id":          "00000000-0000-0000-0000-000000000101",
				"key":         "user.signup",
				"label":       "User Signup",
				"description": "Triggered when a user signs up",
				"active":      true,
				"created_at":  "2024-02-01T09:00:00Z",
				"updated_at":  "2024-02-01T09:30:00Z",
				"created_by":  "user_123",
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.GetWorkspaceEventTypesWithResponse(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if len(*resp.JSON200) != 1 {
		t.Fatalf("expected 1 event type, got %d", len(*resp.JSON200))
	}
	eventType := (*resp.JSON200)[0]
	if eventType.Key != "user.signup" {
		t.Errorf("expected key user.signup, got %s", eventType.Key)
	}
	if eventType.Description == nil || *eventType.Description != "Triggered when a user signs up" {
		t.Fatalf("expected description to be populated, got %v", eventType.Description)
	}
}

func TestGetWorkspaceResources(t *testing.T) {
	workspaceID := mustUUID("00000000-0000-0000-0000-0000000000ae")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/workspaces/"+workspaceID.String()+"/resources" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}

		jsonResponse(t, w, 200, []map[string]interface{}{
			{
				"id":          "00000000-0000-0000-0000-000000000201",
				"key":         "contacts",
				"label":       "Contacts",
				"description": "People synced into downstream systems",
				"active":      true,
				"created_at":  "2024-02-01T09:00:00Z",
				"updated_at":  "2024-02-01T09:45:00Z",
				"created_by":  "user_456",
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.GetWorkspaceResourcesWithResponse(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if len(*resp.JSON200) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(*resp.JSON200))
	}
	resource := (*resp.JSON200)[0]
	if resource.Key != "contacts" {
		t.Errorf("expected key contacts, got %s", resource.Key)
	}
	if resource.Label != "Contacts" {
		t.Errorf("expected label Contacts, got %s", resource.Label)
	}
}

func TestErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(t, w, 404, map[string]interface{}{
			"message": "Workspace not found",
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.GetWorkspaceWithResponse(context.Background(), mustUUID("00000000-0000-0000-0000-000000000099"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode())
	}
	if resp.JSON200 != nil {
		t.Error("expected JSON200 to be nil on 404")
	}
	if resp.JSON404 == nil {
		t.Fatal("expected JSON404 to be non-nil")
	}
	if resp.JSON404.Message != "Workspace not found" {
		t.Errorf("expected 'Workspace not found', got %s", resp.JSON404.Message)
	}
}

func TestEventPayloadAdditionalProperties(t *testing.T) {
	payload := EventPayload{
		Email:     ptrTo(openapi_types.Email("test@example.com")),
		FirstName: ptrTo("John"),
	}
	payload.Set("company", "Acme Corp")
	payload.Set("plan", "enterprise")

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if raw["email"] != "test@example.com" {
		t.Errorf("expected email, got %v", raw["email"])
	}
	if raw["first_name"] != "John" {
		t.Errorf("expected first_name, got %v", raw["first_name"])
	}
	if raw["company"] != "Acme Corp" {
		t.Errorf("expected company, got %v", raw["company"])
	}
	if raw["plan"] != "enterprise" {
		t.Errorf("expected plan, got %v", raw["plan"])
	}

	// Round-trip
	var decoded EventPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	val, found := decoded.Get("company")
	if !found {
		t.Error("expected to find 'company' in additional properties")
	}
	if val != "Acme Corp" {
		t.Errorf("expected 'Acme Corp', got %v", val)
	}
}

func TestGetDefaultMappings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(t, w, 200, map[string]interface{}{
			"status": "active",
			"mapping": map[string]interface{}{
				"id":               "00000000-0000-0000-0000-000000000030",
				"mapping_id":       "00000000-0000-0000-0000-000000000031",
				"connection_id":    "00000000-0000-0000-0000-000000000010",
				"workspace_id":     "00000000-0000-0000-0000-000000000001",
				"integration_type": "hubspot",
				"name":             "Default Mapping",
				"version":          1,
				"created_at":       "2024-01-01T00:00:00Z",
				"updated_at":       "2024-01-01T00:00:00Z",
				"schema": map[string]interface{}{
					"schema_version": 1,
					"fields": []map[string]interface{}{
						{
							"dest":   "email",
							"source": map[string]interface{}{"type": "path", "path": "$.payload.email"},
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := newTestManagementClient(t, server)
	resp, err := client.GetConnectionDefaultMappingsWithResponse(
		context.Background(),
		mustUUID("00000000-0000-0000-0000-000000000010"),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if resp.JSON200.Mapping.Name != "Default Mapping" {
		t.Errorf("expected 'Default Mapping', got %s", resp.JSON200.Mapping.Name)
	}
	if len(resp.JSON200.Mapping.Schema.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(resp.JSON200.Mapping.Schema.Fields))
	}
	if resp.JSON200.Mapping.Schema.Fields[0].Dest != "email" {
		t.Errorf("expected dest 'email', got %s", resp.JSON200.Mapping.Schema.Fields[0].Dest)
	}
}

func TestParseForbiddenResponses(t *testing.T) {
	tests := []struct {
		name  string
		parse func(*http.Response) (interface{}, error)
	}{
		{
			name: "create connection",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseCreateConnectionResponse(rsp)
			},
		},
		{
			name: "delete connection",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseDeleteConnectionResponse(rsp)
			},
		},
		{
			name: "update connection",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseUpdateConnectionResponse(rsp)
			},
		},
		{
			name: "get connection actions",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseGetConnectionActionsResponse(rsp)
			},
		},
		{
			name: "get connection fields",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseGetConnectionFieldsResponse(rsp)
			},
		},
		{
			name: "get default mappings",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseGetConnectionDefaultMappingsResponse(rsp)
			},
		},
		{
			name: "update default mappings",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseUpdateConnectionDefaultMappingsResponse(rsp)
			},
		},
		{
			name: "create rule",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseCreateRuleResponse(rsp)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.parse(newJSONHTTPResponse(http.StatusForbidden, `{"message":"forbidden"}`))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := responseMessageField(t, resp, "JSON403"); got != "forbidden" {
				t.Errorf("expected forbidden message, got %s", got)
			}
		})
	}
}

func TestParseConnectionAdditionalErrorResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		field  string
		parse  func(*http.Response) (interface{}, error)
	}{
		{
			name:   "create connection validation",
			status: http.StatusUnprocessableEntity,
			field:  "JSON422",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseCreateConnectionResponse(rsp)
			},
		},
		{
			name:   "update connection validation",
			status: http.StatusUnprocessableEntity,
			field:  "JSON422",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseUpdateConnectionResponse(rsp)
			},
		},
		{
			name:   "get connection actions bad request",
			status: http.StatusBadRequest,
			field:  "JSON400",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseGetConnectionActionsResponse(rsp)
			},
		},
		{
			name:   "get connection actions conflict",
			status: http.StatusConflict,
			field:  "JSON409",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseGetConnectionActionsResponse(rsp)
			},
		},
		{
			name:   "get connection actions validation",
			status: http.StatusUnprocessableEntity,
			field:  "JSON422",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseGetConnectionActionsResponse(rsp)
			},
		},
		{
			name:   "get connection fields unauthorized",
			status: http.StatusUnauthorized,
			field:  "JSON401",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseGetConnectionFieldsResponse(rsp)
			},
		},
		{
			name:   "get connection fields validation",
			status: http.StatusUnprocessableEntity,
			field:  "JSON422",
			parse: func(rsp *http.Response) (interface{}, error) {
				return ParseGetConnectionFieldsResponse(rsp)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.parse(newJSONHTTPResponse(tt.status, `{"message":"handled"}`))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := responseMessageField(t, resp, tt.field); got != "handled" {
				t.Errorf("expected handled message, got %s", got)
			}
		})
	}
}

// --- Helpers ---

func ptrTo[T any](v T) *T {

	return &v
}
