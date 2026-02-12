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
    client, err := meshes.NewEventClient(
        "https://api.meshes.io",
        "your-publishable-key",
    )
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

### Management API (Bearer JWT Auth)

```go
client, err := meshes.NewManagementClient(
    "https://api.meshes.io",
    "your-jwt-token",
)
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

// List connections for a workspace
connections, err := client.GetWorkspaceConnectionsWithResponse(
    context.Background(),
    workspaceID,
)

// Get rules filtered by event
event := "user.signup"
rules, err := client.GetRulesWithResponse(context.Background(), &meshes.GetRulesParams{
    Event: &event,
})

// Get and update field mappings
mappings, err := client.GetConnectionDefaultMappingsWithResponse(
    context.Background(),
    connectionID,
)
```

## Authentication

| Method | Use Case | Helper |
|--------|----------|--------|
| Bearer JWT | Management APIs (workspaces, connections, rules, mappings) | `NewManagementClient()` |
| Publishable Key | Event ingestion (CreateEvent, CreateBulkEvent) | `NewEventClient()` |

## Response Handling

All `*WithResponse` methods return typed response structs with fields for each possible status code:

```go
resp, err := client.GetWorkspaceWithResponse(ctx, workspaceID)
if err != nil {
    // Network or transport error
    log.Fatal(err)
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
