// send-event demonstrates sending a product event using a publishable key.
//
// Usage:
//
//	cp ../.env.example ../.env   # fill in your credentials
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	meshes "github.com/mesheshq/go-meshes"
	"github.com/mesheshq/go-meshes/examples/internal/envloader"
	openapi_types "github.com/deepmap/oapi-codegen/pkg/types"
)

func ptr[T any](v T) *T { return &v }

func main() {
	// Load .env from the examples directory (walks up one level)
	envloader.Load("../.env")

	publishableKey := envloader.Require("MESHES_PUBLISHABLE_KEY")

	// Build client — optionally override the API URL for local dev
	var opts []meshes.ClientOption
	if apiURL := os.Getenv("MESHES_API_URL"); apiURL != "" {
		opts = append(opts, meshes.WithServerURL(apiURL))
		fmt.Printf("Using custom API URL: %s\n", apiURL)
	}

	client, err := meshes.NewEventClient(publishableKey, opts...)
	if err != nil {
		log.Fatalf("Failed to create event client: %v", err)
	}

	// Build the event payload with known fields + custom properties
	payload := meshes.EventPayload{
		Email:     ptr(openapi_types.Email("jane@example.com")),
		FirstName: ptr("Jane"),
		LastName:  ptr("Doe"),
	}
	payload.Set("plan", "pro")
	payload.Set("company", "Acme Corp")
	payload.Set("signup_source", "landing_page")

	fmt.Println("Sending user.signup event...")

	resp, err := client.CreateEventWithResponse(context.Background(), meshes.CreateEventJSONRequestBody{
		Event:   "user.signup",
		Payload: payload,
	})
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	switch {
	case resp.JSON201 != nil:
		fmt.Printf("Event created successfully!\n")
		fmt.Printf("  ID:        %s\n", resp.JSON201.Event.Id)
		fmt.Printf("  Event:     %s\n", resp.JSON201.Event.Event)
		fmt.Printf("  Workspace: %s\n", resp.JSON201.Event.Workspace)
	case resp.JSON400 != nil:
		fmt.Printf("Bad request: %s\n", resp.JSON400.Message)
	case resp.JSON401 != nil:
		fmt.Printf("Unauthorized: %s\n", resp.JSON401.Message)
	default:
		fmt.Printf("Unexpected status %d: %s\n", resp.StatusCode(), string(resp.Body))
	}
}
