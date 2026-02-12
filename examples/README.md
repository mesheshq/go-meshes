# Examples

Runnable examples for the Meshes Go SDK.

## Setup

```bash
# Copy the env template and fill in your credentials
cp .env.example .env
```

You'll need credentials from [meshes.io](https://meshes.io) → Settings → Machine Keys.

For local development, set `MESHES_API_URL` in your `.env`:

```
MESHES_API_URL=http://localhost:3000
```

## Examples

### send-event

Send a product event using a publishable key:

```bash
cd send-event
go run .
```

### manage-workspaces

List and create workspaces:

```bash
cd manage-workspaces
go run .                          # list workspaces
go run . create "My Workspace"    # create one
```

### create-rule

Wire an event to an integration connection:

```bash
cd create-rule
go run .                                                              # list connections & rules
go run . 550e8400-e29b-41d4-a716-446655440000 user.signup add_contact # create a rule
```
