package workflows

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"

	"github.com/serverlessworkflow/sdk-go/v3/parser"
)

// SimpleWorkflow is a basic workflow definition.
func SimpleWorkflow(ctx workflow.Context, name string) (string, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("SimpleWorkflow workflow started", "name", name)

	// Define activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: time.Second * 5,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var result string
	err := workflow.ExecuteActivity(ctx, SimpleActivity, name).Get(ctx, &result)
	if err != nil {
		return "", err
	}

	logger.Info("SimpleWorkflow workflow completed", "result", result)
	return result, nil
}

// SimpleActivity is a basic activity definition.
func SimpleActivity(ctx context.Context, name string) (string, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("SimpleActivity activity started", "name", name)
	return fmt.Sprintf("Hello, %s!", name), nil
}

// ExecuteServerlessYAMLWorkflow parses and validates the input YAML and returns success if valid.
func ExecuteServerlessYAMLWorkflow(ctx workflow.Context, workflowYAML string) (bool, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("ExecuteServerlessYAMLWorkflow workflow started")

	// Parse and validate the workflow YAML (validation is automatic)
	_, err := parser.FromYAMLSource([]byte(workflowYAML))
	if err != nil {
		logger.Error("Failed to parse serverless workflow YAML", "error", err)
		return false, fmt.Errorf("invalid serverless workflow YAML: %w", err)
	}

	logger.Info("Serverless workflow YAML parsed and validated successfully")
	return true, nil
}

// ExecuteServerlessJSONWorkflow parses and validates the input JSON and returns success if valid.
func ExecuteServerlessJSONWorkflow(ctx workflow.Context, workflowJSON string) (bool, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("ExecuteServerlessJSONWorkflow workflow started")

	// Parse and validate the workflow JSON (validation is automatic)
	_, err := parser.FromJSONSource([]byte(workflowJSON))
	if err != nil {
		logger.Error("Failed to parse serverless workflow JSON", "error", err)
		return false, fmt.Errorf("invalid serverless workflow JSON: %w", err)
	}

	logger.Info("Serverless workflow JSON parsed and validated successfully")
	return true, nil
}
