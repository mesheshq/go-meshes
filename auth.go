package meshes

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const defaultServerURL = "https://api.meshes.io"

// MeshesCredentials holds the machine key credentials for JWT authentication.
type MeshesCredentials struct {
	AccessKey string
	SecretKey string
	OrgID     string
}

// --- JWT minting (HS256) ---
// Meshes uses short-lived HS256 JWTs (max 60s expiry) minted from
// machine key credentials. A fresh token is generated per request.
// See: https://meshes.io/docs/api/authentication

func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func mintJWT(creds MeshesCredentials) (string, error) {
	now := time.Now().UTC()

	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
		"kid": creds.AccessKey,
	}

	payload := map[string]interface{}{
		"iss": fmt.Sprintf("urn:meshes:m2m:%s", creds.AccessKey),
		"aud": "meshes-api",
		"org": creds.OrgID,
		"iat": now.Unix(),
		"exp": now.Add(30 * time.Second).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT header: %w", err)
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT payload: %w", err)
	}

	signingInput := base64URLEncode(headerJSON) + "." + base64URLEncode(payloadJSON)

	mac := hmac.New(sha256.New, []byte(creds.SecretKey))
	mac.Write([]byte(signingInput))
	signature := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

// --- Request editors ---

// withAutoJWT returns a RequestEditorFn that mints a fresh short-lived JWT for each request.
func withAutoJWT(creds MeshesCredentials) RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		token, err := mintJWT(creds)
		if err != nil {
			return fmt.Errorf("failed to mint JWT: %w", err)
		}
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

// --- Client constructors ---

// NewManagementClient creates a client for the management API.
// It automatically mints a fresh short-lived JWT (30s expiry) for each request
// using the provided machine key credentials.
//
// Usage:
//
//	client, err := meshes.NewManagementClient(meshes.MeshesCredentials{
//	    AccessKey: os.Getenv("MESHES_ACCESS_KEY"),
//	    SecretKey: os.Getenv("MESHES_SECRET_KEY"),
//	    OrgID:     os.Getenv("MESHES_ORG_ID"),
//	})
func NewManagementClient(creds MeshesCredentials, opts ...ClientOption) (*ClientWithResponses, error) {
	if creds.AccessKey == "" || creds.SecretKey == "" || creds.OrgID == "" {
		return nil, fmt.Errorf("meshes: AccessKey, SecretKey, and OrgID are all required")
	}
	allOpts := append([]ClientOption{WithRequestEditorFn(withAutoJWT(creds))}, opts...)
	return NewClientWithResponses(defaultServerURL, allOpts...)
}

// NewEventClient creates a client for event ingestion using a publishable key.
//
// Usage:
//
//	client, err := meshes.NewEventClient("pub_live_xxxx")
func NewEventClient(publishableKey string, opts ...ClientOption) (*ClientWithResponses, error) {
	if publishableKey == "" {
		return nil, fmt.Errorf("meshes: publishableKey is required")
	}
	allOpts := append([]ClientOption{WithRequestEditorFn(WithPublishableKey(publishableKey))}, opts...)
	return NewClientWithResponses(defaultServerURL, allOpts...)
}

// WithServerURL overrides the default API server URL (https://api.meshes.io).
func WithServerURL(url string) ClientOption {
	return WithBaseURL(url)
}