package meshes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"github.com/google/uuid"

	openapi_types "github.com/deepmap/oapi-codegen/pkg/types"
)

// --- Helper ---

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

// --- Auth Tests ---

func TestWithBearerAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token-123" {
			t.Errorf("expected Bearer test-token-123, got %s", auth)
		}
		jsonResponse(t, w, 200, map[string]interface{}{
			"count": 0, "limit": 20, "next_cursor": nil, "records": []interface{}{},
		})
	}))
	defer server.Close()

	client, err := NewManagementClient(server.URL, "test-token-123")
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

func TestWithPublishableKey(t *testing.T) {
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

	client, err := NewEventClient(server.URL, "pub_test_key")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

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

// --- Client Tests ---

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

	client, err := NewManagementClient(server.URL, "token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := client.GetWorkspacesWithResponse(context.Background())
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode())
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

	client, err := NewManagementClient(server.URL, "token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

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

	client, err := NewManagementClient(server.URL, "token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	resp, err := client.GetConnectionsWithResponse(context.Background())
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if len(resp.JSON200.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.JSON200.Records))
	}

	conn := resp.JSON200.Records[0]
	if conn.Type != ConnectionTypeHubspot {
		t.Errorf("expected hubspot, got %s", conn.Type)
	}
	if conn.Name != "My HubSpot" {
		t.Errorf("expected 'My HubSpot', got %s", conn.Name)
	}
}

func TestCreateEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

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
		// Verify additional properties are sent
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

	client, err := NewEventClient(server.URL, "pub_test_key")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

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
	if resp.JSON201 == nil {
		t.Fatal("expected JSON201 to be non-nil")
	}
	if resp.JSON201.Event.Event != "user.signup" {
		t.Errorf("expected 'user.signup', got %s", resp.JSON201.Event.Event)
	}
}

func TestGetRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query params are passed
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

	client, err := NewManagementClient(server.URL, "token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	event := "user.signup"
	resp, err := client.GetRulesWithResponse(context.Background(), &GetRulesParams{
		Event: &event,
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatal("expected JSON200 to be non-nil")
	}
	if len(resp.JSON200.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(resp.JSON200.Records))
	}
	if resp.JSON200.Records[0].Metadata.Action != "add_contact" {
		t.Errorf("expected action 'add_contact', got %s", resp.JSON200.Records[0].Metadata.Action)
	}
}

func TestErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(t, w, 404, map[string]interface{}{
			"message": "Workspace not found",
		})
	}))
	defer server.Close()

	client, err := NewManagementClient(server.URL, "token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

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

	client, err := NewManagementClient(server.URL, "token")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

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

// --- Helpers ---

func ptrTo[T any](v T) *T {
	return &v
}
