# go-meshes

Official Go SDK for the [Meshes](https://meshes.io) API.

## Installation

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

func ptr[T any](v T) *T { return &v }
```

### Management API (Machine Key Auth)

The management client automatically mints short-lived HS256 JWTs (30s expiry) for each request using your machine key credentials. No manual token management needed.

```go
import (
    "os"
    meshes "github.com/mesheshq/go-meshes"
)

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
for _, ws := range workspaces.JSON200.Records {
    fmt.Printf("Workspace: %s (%s)\n", ws.Name, ws.Id)
}

// List connections, get rules, manage mappings, etc.
connections, _ := client.GetWorkspaceConnectionsWithResponse(ctx, workspaceID)

event := "user.signup"
rules, _ := client.GetRulesWithResponse(ctx, &meshes.GetRulesParams{Event: &event})

mappings, _ := client.GetConnectionDefaultMappingsWithResponse(ctx, connectionID)
```

## Authentication

| Method | Use Case | Constructor |
|--------|----------|-------------|
| Machine Key (HS256 JWT) | Management APIs (workspaces, connections, rules, mappings) | `NewManagementClient(creds)` |
| Publishable Key | Event ingestion (CreateEvent, CreateBulkEvent) | `NewEventClient(key)` |

The management client handles JWT generation automatically — each request gets a fresh token signed with your secret key. Tokens expire in 30 seconds. See [Authentication docs](https://meshes.io/docs/api/authentication) for details.

## Configuration

The SDK defaults to `https://api.meshes.io`. To override (e.g. for local development):

```go
client, err := meshes.NewManagementClient(creds, meshes.WithServerURL("http://localhost:3000"))
```

## Response Handling

All `*WithResponse` methods return typed response structs with fields for each possible status code:

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
}
```

## Generated from OpenAPI

This SDK is generated from the Meshes OpenAPI specification using [oapi-codegen](https://github.com/deepmap/oapi-codegen).

## License

MIT
