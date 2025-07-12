# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

### Prerequisites Setup
```bash
# Start Temporal server (required)
cd temporal-docker && docker-compose up -d

# Set environment variable
export ANTHROPIC_API_KEY="your-anthropic-api-key"
```

### Build and Run
```bash
# Install dependencies
go mod download

# Run the server (starts on :8088)
go run cmd/api/main.go

# Build binary
go build -o bin/server cmd/api/main.go

# Run tests
go test ./...

# Run specific test
go test ./internal/workflows -v -run TestSpecificFunction
```

### Lint and Format
```bash
# Format code
go fmt ./...

# Vet code
go vet ./...

# Run linter (if golangci-lint is available)
golangci-lint run
```

## Architecture Overview

This is a **Temporal-based workflow orchestration backend** that combines serverless workflow execution with chatbot functionality. The system uses Temporal as the core orchestration engine with two primary workflow patterns:

### Core Architecture Pattern
```
HTTP API Layer → Temporal Workflows → External Services (Anthropic Claude)
```

**Key Components:**
- **`cmd/api/main.go`**: Application entry point that initializes Temporal client, starts worker, and HTTP server
- **`internal/api/handlers.go`**: REST API handlers that interact with Temporal client
- **`internal/workflows/`**: Temporal workflow and activity definitions

### Workflow Patterns

1. **Request-Response Workflows** (`ExecuteServerlessYAMLWorkflow`):
   - Stateless execution of serverless workflow specifications
   - Used for batch processing and validation

2. **Long-Running Signal Workflows** (`ChatbotWorkflow`):
   - Persistent conversation state maintained in workflow
   - Uses Temporal signals for user input
   - Supports queries for retrieving conversation history
   - Integrates with Anthropic Claude API via activities

### Critical Dependencies

- **Temporal SDK**: All workflow orchestration runs through `go.temporal.io/sdk`
- **Anthropic SDK**: Chatbot functionality requires `github.com/anthropics/anthropic-sdk-go`
- **Serverless Workflow SDK**: Validates workflow specs with `github.com/serverlessworkflow/sdk-go/v3`

### State Management

**Chatbot State**: Conversation history stored as `[]anthropic.MessageParam` in workflow state, accessible via Temporal queries.

**Workflow IDs**: Follow pattern `{type}-workflow-{uuid}` (e.g., `chatbot-workflow-abc-123`).

## Development Guidelines

### Adding New Workflows
1. Define workflow function in `internal/workflows/`
2. Register in `worker.go` using `w.RegisterWorkflow()`
3. Add corresponding API handler in `handlers.go`
4. Use consistent naming pattern for workflow IDs

### Chatbot Modifications
- Conversation state is managed entirely within Temporal workflow
- API calls to Anthropic are handled in `ChatbotActivities`
- Use signals for user input and queries for state retrieval
- Activity timeouts are set to 30 seconds for API calls

### Testing Temporal Workflows
- Temporal provides test frameworks in `go.temporal.io/sdk/testsuite`
- Mock external API calls in activities for unit testing
- Test signal and query handlers separately

### Environment Configuration
- `ANTHROPIC_API_KEY`: Required for chatbot functionality
- Temporal server: Assumes localhost:7233 (default Docker setup)
- HTTP server: Runs on port 8088

### Common Pitfalls
- Temporal workflows must be deterministic - avoid random values, current time, or external API calls in workflow code
- Activities handle all non-deterministic operations (API calls, random generation)
- Workflow state changes require proper serialization support
- Signal names must match between sender (API) and receiver (workflow)