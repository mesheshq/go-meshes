// create-rule demonstrates connecting a product event to an integration.
// This is the core Meshes workflow: when "user.signup" fires, automatically
// add the contact in HubSpot (or whichever connection you choose).
//
// Usage:
//
//	cp ../.env.example ../.env   # fill in your credentials
//	go run .                                              # interactive — lists connections, then creates rule
//	go run . <connection_id> <event_name> <action>        # direct
//
// Example:
//
//	go run . 550e8400-e29b-41d4-a716-446655440000 user.signup add_contact
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	meshes "github.com/mesheshq/go-meshes"
	"github.com/mesheshq/go-meshes/examples/internal/envloader"

	"github.com/google/uuid"
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

	// Direct mode: go run . <connection_id> <event> <action>
	if len(os.Args) == 4 {
		connID, err := uuid.Parse(os.Args[1])
		if err != nil {
			log.Fatalf("Invalid connection ID: %v", err)
		}
		createRule(ctx, client, connID, os.Args[2], os.Args[3])
		return
	}

	// Interactive mode: show connections, then list existing rules
	showConnectionsAndRules(ctx, client)
}

func showConnectionsAndRules(ctx context.Context, client *meshes.ClientWithResponses) {
	// Step 1: List connections
	fmt.Println("Fetching connections...")
	connResp, err := client.GetConnectionsWithResponse(ctx)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	switch {
	case connResp.JSON200 != nil:
		connections := connResp.JSON200.Records
		if connResp.JSON200.Count == 0 {
			fmt.Println("No connections found. Create a connection in the Meshes dashboard first.")
			return
		}

		fmt.Printf("Found %d connection(s):\n\n", connResp.JSON200.Count)
		for _, conn := range connections {
			fmt.Printf("  %-36s  %-12s  %s\n", conn.Id, conn.Type, conn.Name)
		}
		fmt.Println()
	default:
		fmt.Printf("Unexpected status %d: %s\n", connResp.StatusCode(), string(connResp.Body))
		return
	}

	// Step 2: List existing rules
	fmt.Println("Fetching rules...")
	rulesResp, err := client.GetRulesWithResponse(ctx, &meshes.GetRulesParams{})
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	switch {
	case rulesResp.JSON200 != nil:
		rules := rulesResp.JSON200.Records
		if rulesResp.JSON200.Count == 0 {
			fmt.Println("No rules yet. Create one with:")
			fmt.Println("  go run . <connection_id> <event_name> <action>")
			fmt.Println()
			fmt.Println("Example:")
			fmt.Println("  go run . 550e8400-e29b-41d4-a716-446655440000 user.signup add_contact")
			return
		}

		fmt.Printf("Found %d rule(s):\n\n", rulesResp.JSON200.Count)
		for _, rule := range rules {
			activeStr := "inactive"
			if rule.Active != nil && *rule.Active {
				activeStr = "active"
			}
			fmt.Printf("  %-36s  %-20s  %-12s  [%s]\n",
				rule.Id, rule.Event, rule.Metadata.Action, activeStr)
		}
	default:
		fmt.Printf("Unexpected status %d: %s\n", rulesResp.StatusCode(), string(rulesResp.Body))
	}
}

func createRule(ctx context.Context, client *meshes.ClientWithResponses, connectionID uuid.UUID, event string, action string) {
	fmt.Printf("Creating rule: %s → %s (connection %s)...\n", event, action, connectionID)

	resp, err := client.CreateRuleWithResponse(ctx, meshes.CreateRuleJSONRequestBody{
		Connection: connectionID,
		Event:      event,
		Metadata: meshes.RuleMetadata{
			Action: action,
		},
	})
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}

	switch {
	case resp.JSON201 != nil:
		rule := resp.JSON201.Rule
		fmt.Printf("Rule created!\n")
		fmt.Printf("  ID:         %s\n", rule.Id)
		fmt.Printf("  Event:      %s\n", rule.Event)
		fmt.Printf("  Connection: %s\n", rule.Connection)
		fmt.Printf("  Action:     %s\n", rule.Metadata.Action)
		fmt.Printf("  Active:     %v\n", rule.Active)
		fmt.Println()
		fmt.Printf("When a %q event is sent, it will now trigger %q on this connection.\n", event, action)
	default:
		fmt.Printf("Unexpected status %d: %s\n", resp.StatusCode(), string(resp.Body))
	}
}