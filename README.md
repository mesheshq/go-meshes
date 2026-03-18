# go-meshes

Official Go SDK for the [Meshes](https://meshes.io) API.

## Installation

Requires **Go 1.25** or later (recommended). The SDK uses generics and may work with Go 1.18+, but is only tested against the latest supported Go release.

```bash
go get github.com/mesheshq/go-meshes
```

## Quick Start

### Sending Events (Publishable Key Auth)

```go
package main

import (
    "context"
    "fmt"
    "log"

    meshes "github.com/mesheshq/go-meshes"
    openapi_types "github.com/deepmap/oapi-codegen/pkg/types"
)

func main() {
    client, err := meshes.NewEventClient("your-publishable-key")
    if err != nil {
        log.Fatal(err)
    }

    payload := meshes.EventPayload{
        Email:     ptr(openapi_types.Email("jane@example.com")),
        FirstName: ptr("Jane"),
        LastName:  ptr("Doe"),
    }
    // Add any custom fields
    payload.Set("plan", "pro")
    payload.Set("company", "Acme Corp")

    resp, err := client.CreateEventWithResponse(context.Background(), meshes.CreateEventJSONRequestBody{
        Event:   "user.signup",
        Payload: payload,
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Event created: %s\n", resp.JSON201.Event.Id)
}

// ptr is a helper that returns a pointer to the given value.
// The generated OpenAPI types use pointers for optional fields,
// so this avoids needing temporary variables for every field.
func ptr[T any](v T) *T { return &v }
```

### Sending Batch Events

Batch requests support up to **100 events** per call:

```go
resp, err := client.CreateBulkEventWithResponse(context.Background(), meshes.CreateBulkEventJSONRequestBody{
    Events: []meshes.EventInput{
        {
            Event: "user.signup",
            Payload: meshes.EventPayload{
                Email:     ptr(openapi_types.Email("jane@example.com")),
                FirstName: ptr("Jane"),
            },
        },
        {
            Event: "membership.started",
            Payload: meshes.EventPayload{
                Email: ptr(openapi_types.Email("jane@example.com")),
            },
        },
    },
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Batch accepted: %d events\n", len(resp.JSON201.Events))
```

### Management API (Machine Key Auth)

The management client automatically mints short-lived HS256 JWTs (30s expiry) for each request using your machine key credentials. No manual token management needed.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    meshes "github.com/mesheshq/go-meshes"
)

func main() {
    client, err := meshes.NewManagementClient(meshes.MeshesCredentials{
        AccessKey: os.Getenv("MESHES_ACCESS_KEY"),
        SecretKey: os.Getenv("MESHES_SECRET_KEY"),
        OrgID:     os.Getenv("MESHES_ORG_ID"),
    })
    if err != nil {
        log.Fatal(err)
    }

    // List workspaces
    workspaces, err := client.GetWorkspacesWithResponse(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    for _, ws := range workspaces.JSON200.Records {
        fmt.Printf("Workspace: %s (%s)\n", ws.Name, ws.Id)
    }

    // List connections, get rules, inspect workspace metadata, manage sessions, etc.
    connections, _ := client.GetWorkspaceConnectionsWithResponse(context.Background(), workspaceID)
    eventTypes, _ := client.GetWorkspaceEventTypesWithResponse(context.Background(), workspaceID)
    resources, _ := client.GetWorkspaceResourcesWithResponse(context.Background(), workspaceID)

    event := "user.signup"
    rules, _ := client.GetRulesWithResponse(context.Background(), &meshes.GetRulesParams{Event: &event})

    mappings, _ := client.GetConnectionDefaultMappingsWithResponse(context.Background(), connectionID)

    _ = connections
    _ = eventTypes
    _ = resources
    _ = rules
    _ = mappings
}
```

### Creating And Managing Sessions

Use the management client to create short-lived workspace sessions, then refresh or revoke them later by `session_id`:

```go
role := meshes.Member
allowedOrigins := []string{"https://app.example.com"}
scopes := []meshes.CreateSessionJSONBodyScopes{meshes.EventsPayloadRead}
launchPath := "/dashboard"

session, err := client.CreateSessionWithResponse(context.Background(), meshes.CreateSessionJSONRequestBody{
    WorkspaceId:    workspaceID,
    AllowedOrigins: &allowedOrigins,
    LaunchPath:     &launchPath,
    Role:           &role,
    Scopes:         &scopes,
})
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Launch URL: %s\n", session.JSON200.LaunchUrl)

sessions, _ := client.ListSessionsWithResponse(context.Background(), &meshes.ListSessionsParams{
    WorkspaceId: workspaceID,
})

if len(sessions.JSON200.Records) > 0 {
    _, _ = client.RefreshSessionWithResponse(context.Background(), sessions.JSON200.Records[0].SessionId)
    _, _ = client.RevokeSessionWithResponse(context.Background(), sessions.JSON200.Records[0].SessionId)
}
```

## Retries

This SDK is a **thin client**: it sends events to Meshes and does **not** retry on its own. Once Meshes accepts an event (2xx response), retries and delivery guarantees are handled server-side by the Meshes platform.

This means:

- Your application code stays simple with no retry loops or backoff logic
- Meshes handles retrying failed deliveries to downstream integrations (HubSpot, Salesforce, etc.) with exponential backoff
- If the initial request to Meshes fails (network error, 5xx), the SDK returns an error that you can handle as needed

## Authentication

| Method | Use Case | Constructor |
|--------|----------|-------------|
| Machine Key (HS256 JWT) | Management APIs (workspaces, connections, rules, mappings, sessions) | `NewManagementClient(creds)` |
| Publishable Key | Event ingestion (CreateEvent, CreateBulkEvent) | `NewEventClient(key)` |

The management client handles JWT generation automatically; each request gets a fresh token signed with your secret key. Tokens expire in 30 seconds. See [Authentication docs](https://meshes.io/docs/api/authentication) for details.

## Configuration

The SDK defaults to `https://api.meshes.io`. To override (for local development, for example):

```go
client, err := meshes.NewManagementClient(creds, meshes.WithServerURL("http://localhost:3000"))
```

## Timeouts

Use Go's standard `context.WithTimeout` to set request deadlines:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

resp, err := client.CreateEventWithResponse(ctx, meshes.CreateEventJSONRequestBody{
    Event:   "user.signup",
    Payload: payload,
})
if err != nil {
    // may be a context.DeadlineExceeded error
    log.Fatal(err)
}
```

## Error Handling

The SDK returns two kinds of errors:

**Transport errors**: returned as a standard Go `error` when the request fails at the network level (DNS, connection refused, timeout, etc.):

```go
resp, err := client.CreateEventWithResponse(ctx, body)
if err != nil {
    // Network or transport error: request never reached Meshes
    log.Fatal(err)
}
```

**API errors**: when the request reaches Meshes but returns a non-2xx status. All `*WithResponse` methods return typed response structs with fields for each possible status code:

```go
resp, err := client.GetWorkspaceWithResponse(ctx, workspaceID)
if err != nil {
    log.Fatal(err) // network or transport error
}

switch {
case resp.JSON200 != nil:
    fmt.Println("Found:", resp.JSON200.Name)
case resp.JSON404 != nil:
    fmt.Println("Not found:", resp.JSON404.Message)
case resp.JSON500 != nil:
    fmt.Println("Server error:", resp.JSON500.Message)
default:
    fmt.Printf("Unexpected status: %d\n", resp.StatusCode())
}
```

## Dependencies

This SDK is generated from the Meshes OpenAPI specification using [oapi-codegen](https://github.com/deepmap/oapi-codegen). As a result, event payloads use `openapi_types.Email` from the `oapi-codegen` package for the email field type. This is re-exported through the SDK's `EventPayload` struct; you only need to import it directly when constructing payloads with typed email fields.

## License

MIT
