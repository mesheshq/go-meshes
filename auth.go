package meshes

import (
	"context"
	"fmt"
	"net/http"
)

// WithBearerAuth returns a RequestEditorFn that adds a Bearer token to requests.
// Use this for management API calls (workspaces, connections, rules, etc.)
func WithBearerAuth(token string) RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		return nil
	}
}

// WithPublishableKey returns a RequestEditorFn that adds a publishable key header.
// Use this for event ingestion (CreateEvent, CreateBulkEvent).
func WithPublishableKey(key string) RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		req.Header.Set("X-Meshes-Publishable-Key", key)
		return nil
	}
}

// NewManagementClient creates a client configured for management API access with Bearer JWT auth.
func NewManagementClient(serverURL string, token string, opts ...ClientOption) (*ClientWithResponses, error) {
	allOpts := append([]ClientOption{WithRequestEditorFn(WithBearerAuth(token))}, opts...)
	return NewClientWithResponses(serverURL, allOpts...)
}

// NewEventClient creates a client configured for event ingestion with a publishable key.
func NewEventClient(serverURL string, publishableKey string, opts ...ClientOption) (*ClientWithResponses, error) {
	allOpts := append([]ClientOption{WithRequestEditorFn(WithPublishableKey(publishableKey))}, opts...)
	return NewClientWithResponses(serverURL, allOpts...)
}
