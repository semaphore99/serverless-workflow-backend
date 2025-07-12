# Serverless Workflow Backend Demo

A demonstration serverless workflow backend that integrates a chatbot with workflow execution capabilities using Temporal as the workflow engine.

## Overview

This demo showcases how to build a backend service that combines:
- **Serverless Workflow execution** using the [Serverless Workflow specification](https://serverlessworkflow.io/)
- **Chatbot functionality** powered by Anthropic's Claude AI
- **Temporal workflow engine** for reliable workflow orchestration
- **RESTful API** for workflow and chatbot operations

## Features

- 🤖 **Interactive Chatbot**: Chat with Claude AI through persistent conversation threads
- 🔄 **Workflow Execution**: Execute serverless workflows defined in JSON/YAML
- ⚡ **Temporal Integration**: Robust workflow orchestration with Temporal
- 🏥 **Health Monitoring**: Built-in health check endpoints
- 📡 **RESTful API**: Simple HTTP API for all operations

## Prerequisites

Before running this demo, ensure you have:

1. **Go 1.24+** installed
2. **Temporal Server** running locally
3. **Anthropic API Key** for Claude integration

### Setting up Temporal Server

The easiest way to run Temporal locally is using Docker:

```bash
# Clone Temporal's docker-compose setup
git clone https://github.com/temporalio/docker-compose.git temporal-docker
cd temporal-docker

# Start Temporal server
docker-compose up -d

# Verify it's running (should show Temporal UI at http://localhost:8080)
```

### Getting an Anthropic API Key

1. Sign up at [Anthropic Console](https://console.anthropic.com/)
2. Create an API key
3. Set it as an environment variable (see Configuration section)

## Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/semaphore99/serverless-workflow-backend.git
   cd serverless-workflow-backend
   ```

2. **Install dependencies:**
   ```bash
   go mod download
   ```

3. **Set up environment variables:**
   ```bash
   # Create environment file
   export ANTHROPIC_API_KEY="your-anthropic-api-key-here"
   
   # Or create a .env file (not recommended for production)
   echo "ANTHROPIC_API_KEY=your-anthropic-api-key-here" > .env
   ```

## Usage

### Starting the Server

```bash
# Build and run the server
go run cmd/api/main.go
```

The server will start on `http://localhost:8088` and automatically:
- Connect to the local Temporal server
- Start a Temporal worker
- Expose REST API endpoints

### API Endpoints

#### Health Check
```bash
GET http://localhost:8088/health
```

#### Workflow Operations
```bash
# Execute a serverless workflow
POST http://localhost:8088/workflows
Content-Type: application/json

{
  "name": "example-workflow",
  "version": "1.0",
  "states": [...]
}
```

#### Chatbot Operations
```bash
# Initialize a new chat thread
POST http://localhost:8088/chatbot/init

# Send a message to a chat thread
POST http://localhost:8088/chatbot/message
Content-Type: application/json

{
  "thread_id": "your-thread-id",
  "message": "Hello, how can you help me?"
}

# Get chat thread history
GET http://localhost:8088/chatbot/thread?thread_id=your-thread-id
```

### Example Usage

1. **Start a chat session:**
   ```bash
   curl -X POST http://localhost:8088/chatbot/init
   # Returns: {"workflow_id": "...", "thread_id": "abc-123"}
   ```

2. **Send a message:**
   ```bash
   curl -X POST http://localhost:8088/chatbot/message \
     -H "Content-Type: application/json" \
     -d '{"thread_id": "abc-123", "message": "What is a serverless workflow?"}'
   ```

3. **Get conversation history:**
   ```bash
   curl "http://localhost:8088/chatbot/thread?thread_id=abc-123"
   ```

## Project Structure

```
├── cmd/api/main.go                 # Application entry point
├── internal/
│   ├── api/handlers.go            # HTTP request handlers
│   └── workflows/
│       ├── ChatbotWorkflow.go     # Chatbot workflow implementation
│       ├── workflow.go            # Serverless workflow execution
│       └── worker.go              # Temporal worker setup
├── sample.sw.json                 # Example serverless workflow (JSON)
├── sample.sw.yaml                 # Example serverless workflow (YAML)
├── go.mod                         # Go module definition
└── README.md                      # This file
```

## Configuration

The application uses environment variables for configuration:

| Variable | Description | Required |
|----------|-------------|----------|
| `ANTHROPIC_API_KEY` | Your Anthropic API key for Claude | Yes |

## Demo Limitations

⚠️ **This is a demonstration project intended for local development only:**

- **No Authentication**: All endpoints are publicly accessible
- **No Persistence**: Data is lost when the server restarts
- **Local Only**: Assumes Temporal server running on localhost
- **Basic Error Handling**: Minimal production-ready error handling
- **No Rate Limiting**: No protection against API abuse

## Development

### Running Tests
```bash
go test ./...
```

### Building for Production
```bash
go build -o bin/server cmd/api/main.go
```

### Adding New Workflows
1. Define your workflow in `internal/workflows/`
2. Register it in `worker.go`
3. Add corresponding API handlers if needed

## Technologies Used

- **[Go](https://golang.org/)** - Backend language
- **[Temporal](https://temporal.io/)** - Workflow orchestration engine
- **[Anthropic SDK](https://github.com/anthropics/anthropic-sdk-go)** - Claude AI integration
- **[Serverless Workflow](https://serverlessworkflow.io/)** - Workflow specification

## Contributing

This is a demo project, but contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is for demonstration purposes. See LICENSE file for details.

## Support

For questions or issues:
- Open an issue on GitHub
- Check the [Temporal documentation](https://docs.temporal.io/)
- Review [Serverless Workflow specification](https://serverlessworkflow.io/)