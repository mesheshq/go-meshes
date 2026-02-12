// manage-workspaces demonstrates listing and creating workspaces
// using machine key credentials for JWT authentication.
//
// Usage:
//
//	cp ../.env.example ../.env   # fill in your credentials
//	go run .
//	go run . create "My New Workspace"
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	meshes "github.com/mesheshq/go-meshes"
	"github.com/mesheshq/go-meshes/examples/internal/envloader"
)

func main() {
	envloader.Load("../.env")

	creds := meshes.MeshesCredentials{
		AccessKey: envloader.Require("MESHES_ACCESS_KEY"),
		SecretKey: envloader.Require("MESHES_SECRET_KEY"),
		OrgID:     envloader.Require("MESHES_ORG_ID"),
	}

	var opts []meshes.ClientOption
	if apiURL := os.Getenv("MESHES_API_URL"); apiURL != "" {
		opts = append(opts, meshes.WithServerURL(apiURL))
		fmt.Printf("Using custom API URL: %s\n", apiURL)
	}

	client, err := meshes.NewManagementClient(creds, opts...)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()

	// Subcommand: "create <name>"
	if len(os.Args) >= 3 && os.Args[1] == "create" {
		createWorkspace(ctx, client, os.Args[2])
		return
	}

	// Default: list workspaces
	listWorkspaces(ctx, client)
}

func listWorkspaces(ctx context.Context, client *meshes.ClientWithResponses) {
	fmt.Println("Fetching workspaces...")

	resp, err := client.GetWorkspacesWithResponse(ctx)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	switch {
	case resp.JSON200 != nil:
		records := resp.JSON200.Records
		fmt.Printf("Found %d workspace(s):\n\n", resp.JSON200.Count)
		for _, ws := range records {
			fmt.Printf("  %-36s  %s\n", ws.Id, ws.Name)
			fmt.Printf("  %-36s  created %s\n", "", ws.CreatedAt.Format("2006-01-02"))
			fmt.Println()
		}

		if resp.JSON200.Count == 0 {
			fmt.Println("  No workspaces yet. Create one with:")
			fmt.Println("    go run . create \"My Workspace\"")
		}
	default:
		fmt.Printf("Unexpected status %d: %s\n", resp.StatusCode(), string(resp.Body))
	}
}

func createWorkspace(ctx context.Context, client *meshes.ClientWithResponses, name string) {
	fmt.Printf("Creating workspace %q...\n", name)

	resp, err := client.CreateWorkspaceWithResponse(ctx, meshes.CreateWorkspaceJSONRequestBody{
		Name: name,
	})
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	switch {
	case resp.JSON201 != nil:
		ws := resp.JSON201.Workspace
		fmt.Printf("Workspace created!\n")
		fmt.Printf("  ID:   %s\n", ws.Id)
		fmt.Printf("  Name: %s\n", ws.Name)
	default:
		fmt.Printf("Unexpected status %d: %s\n", resp.StatusCode(), string(resp.Body))
	}
}