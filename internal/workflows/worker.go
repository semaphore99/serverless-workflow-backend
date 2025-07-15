package workflows

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// StartWorker starts the Temporal worker and returns the worker instance.
func StartWorker(c client.Client) worker.Worker {
	w := worker.New(c, "serverless-workflow-task-queue", worker.Options{})

	w.RegisterWorkflow(SimpleWorkflow)
	w.RegisterWorkflow(ExecuteServerlessYAMLWorkflow)
	w.RegisterWorkflow(ExecuteServerlessJSONWorkflow)
	w.RegisterWorkflow(ChatbotWorkflow)
	w.RegisterActivity(SimpleActivity)

	// Register chatbot activities
	chatbotActivities := NewChatbotActivities()
	w.RegisterActivity(chatbotActivities.CallClaudeAPI)

	// Register serverless workflow activities
	w.RegisterActivity(HTTPCallActivity)
	w.RegisterActivity(ExecuteBranchActivity)

	return w
}
