package workflows

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"

	"github.com/serverlessworkflow/sdk-go/v3/model"
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

// ExecuteServerlessJSONWorkflow parses, validates, and executes the serverless workflow JSON.
func ExecuteServerlessJSONWorkflow(ctx workflow.Context, workflowJSON string) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("ExecuteServerlessJSONWorkflow workflow started")

	// Parse and validate the workflow JSON (validation is automatic)
	workflowDef, err := parser.FromJSONSource([]byte(workflowJSON))
	if err != nil {
		logger.Error("Failed to parse serverless workflow JSON", "error", err)
		return nil, fmt.Errorf("invalid serverless workflow JSON: %w", err)
	}

	logger.Info("Serverless workflow JSON parsed and validated successfully")

	// Execute the workflow
	result, err := executeWorkflowDefinition(ctx, workflowDef)
	if err != nil {
		logger.Error("Failed to execute serverless workflow", "error", err)
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	logger.Info("Serverless workflow executed successfully")
	return result, nil
}

// executeWorkflowDefinition steps through the workflow definition and executes tasks
func executeWorkflowDefinition(ctx workflow.Context, workflowDef *model.Workflow) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	
	// Set up activity options
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: time.Second * 30,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Initialize workflow state
	workflowState := make(map[string]interface{})
	
	// Execute the "do" tasks
	if workflowDef.Do != nil {
		result, err := executeTasks(ctx, *workflowDef.Do, workflowState)
		if err != nil {
			return nil, err
		}
		workflowState["result"] = result
	}

	logger.Info("Workflow execution completed", "state", workflowState)
	return workflowState, nil
}

// executeTasks executes a list of tasks sequentially
func executeTasks(ctx workflow.Context, tasks model.TaskList, state map[string]interface{}) (interface{}, error) {
	logger := workflow.GetLogger(ctx)
	var lastResult interface{}

	for i, taskItem := range tasks {
		logger.Info("Executing task", "index", i, "key", taskItem.Key)
		
		result, err := executeTaskItem(ctx, taskItem, state)
		if err != nil {
			return nil, fmt.Errorf("task %d (%s) failed: %w", i, taskItem.Key, err)
		}
		
		lastResult = result
		state[fmt.Sprintf("task_%d_result", i)] = result
		state[taskItem.Key] = result
	}

	return lastResult, nil
}

// executeTaskItem executes a single task item
func executeTaskItem(ctx workflow.Context, taskItem *model.TaskItem, state map[string]interface{}) (interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Executing task item", "key", taskItem.Key)

	// Handle different task types using the As* methods
	if httpTask := taskItem.AsCallHTTPTask(); httpTask != nil {
		return executeHTTPTask(ctx, httpTask, state)
	}
	if forkTask := taskItem.AsForkTask(); forkTask != nil {
		return executeForkTaskItem(ctx, forkTask, state)
	}
	if setTask := taskItem.AsSetTask(); setTask != nil {
		return executeSetTaskItem(setTask, state)
	}
	if doTask := taskItem.AsDoTask(); doTask != nil {
		return executeDoTask(ctx, doTask, state)
	}

	return nil, fmt.Errorf("unsupported task type for task: %s", taskItem.Key)
}

// executeHTTPTask handles HTTP calls
func executeHTTPTask(ctx workflow.Context, httpTask *model.CallHTTP, state map[string]interface{}) (interface{}, error) {
	logger := workflow.GetLogger(ctx)
	
	// Execute HTTP call via activity
	var result HTTPCallResult
	err := workflow.ExecuteActivity(ctx, HTTPCallActivity, HTTPCallRequest{
		Method:   httpTask.With.Method,
		Endpoint: httpTask.With.Endpoint.String(),
		Body:     httpTask.With.Body,
		Headers:  httpTask.With.Headers,
	}).Get(ctx, &result)
	
	if err != nil {
		return nil, fmt.Errorf("HTTP call failed: %w", err)
	}
	
	logger.Info("HTTP call completed", "status", result.Status, "endpoint", httpTask.With.Endpoint.String())
	return result, nil
}

// executeForkTaskItem handles parallel execution
func executeForkTaskItem(ctx workflow.Context, forkTask *model.ForkTask, state map[string]interface{}) (interface{}, error) {
	logger := workflow.GetLogger(ctx)
	
	if forkTask.Fork.Branches == nil || len(*forkTask.Fork.Branches) == 0 {
		return nil, fmt.Errorf("fork task has no branches")
	}
	
	// Execute branches in parallel
	branches := *forkTask.Fork.Branches
	futures := make([]workflow.Future, len(branches))
	for i, branch := range branches {
		futures[i] = workflow.ExecuteActivity(ctx, ExecuteBranchActivity, ExecuteBranchRequest{
			Tasks: model.TaskList{branch},
			State: state,
		})
	}
	
	// Wait for all branches to complete
	results := make([]interface{}, len(futures))
	for i, future := range futures {
		var result interface{}
		err := future.Get(ctx, &result)
		if err != nil {
			return nil, fmt.Errorf("branch %d failed: %w", i, err)
		}
		results[i] = result
	}
	
	logger.Info("Fork task completed", "branches", len(results))
	return results, nil
}

// executeSetTaskItem handles variable assignment
func executeSetTaskItem(setTask *model.SetTask, state map[string]interface{}) (interface{}, error) {
	for key, value := range setTask.Set {
		state[key] = value
	}
	return setTask.Set, nil
}

// executeDoTask handles sequential execution of nested tasks
func executeDoTask(ctx workflow.Context, doTask *model.DoTask, state map[string]interface{}) (interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Executing do task with nested tasks", "taskCount", len(*doTask.Do))
	
	// Execute nested tasks sequentially
	return executeTasks(ctx, *doTask.Do, state)
}

// HTTPCallRequest represents an HTTP call request
type HTTPCallRequest struct {
	Method   string                 `json:"method"`
	Endpoint string                 `json:"endpoint"`
	Body     interface{}           `json:"body"`
	Headers  map[string]string     `json:"headers"`
}

// HTTPCallResult represents an HTTP call result
type HTTPCallResult struct {
	Status   int                    `json:"status"`
	Body     interface{}           `json:"body"`
	Headers  map[string]string     `json:"headers"`
}

// ExecuteBranchRequest represents a branch execution request
type ExecuteBranchRequest struct {
	Tasks model.TaskList         `json:"tasks"`
	State map[string]interface{} `json:"state"`
}

// HTTPCallActivity executes HTTP calls
func HTTPCallActivity(ctx context.Context, req HTTPCallRequest) (HTTPCallResult, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("HTTPCallActivity started", "method", req.Method, "endpoint", req.Endpoint)

	// Check if this is a temporal endpoint
	if strings.HasPrefix(req.Endpoint, "http://localhost:8088/temporal") {
		return executeTemporalCall(ctx, req)
	}

	// For other endpoints, make regular HTTP calls
	return executeRegularHTTPCall(ctx, req)
}

// executeTemporalCall handles calls to temporal endpoints
func executeTemporalCall(ctx context.Context, req HTTPCallRequest) (HTTPCallResult, error) {
	logger := activity.GetLogger(ctx)
	
	// Parse the request body to determine if it's a workflow or activity call
	bodyBytes, err := json.Marshal(req.Body)
	if err != nil {
		return HTTPCallResult{}, fmt.Errorf("failed to marshal request body: %w", err)
	}
	
	var callRequest struct {
		WorkflowType string                 `json:"workflowType"`
		ActivityType string                 `json:"activityType"`
		Input        map[string]interface{} `json:"input"`
	}
	
	if err := json.Unmarshal(bodyBytes, &callRequest); err != nil {
		return HTTPCallResult{}, fmt.Errorf("failed to parse temporal call request: %w", err)
	}
	
	if callRequest.WorkflowType != "" {
		logger.Info("Executing child workflow", "workflowType", callRequest.WorkflowType)
		// TODO: Execute child workflow
		return HTTPCallResult{
			Status: 200,
			Body:   map[string]interface{}{"result": "Child workflow executed", "workflowType": callRequest.WorkflowType},
		}, nil
	}
	
	if callRequest.ActivityType != "" {
		logger.Info("Executing activity", "activityType", callRequest.ActivityType)
		// TODO: Execute activity
		return HTTPCallResult{
			Status: 200,
			Body:   map[string]interface{}{"result": "Activity executed", "activityType": callRequest.ActivityType},
		}, nil
	}
	
	return HTTPCallResult{}, fmt.Errorf("unknown temporal call type")
}

// executeRegularHTTPCall handles regular HTTP calls
func executeRegularHTTPCall(ctx context.Context, req HTTPCallRequest) (HTTPCallResult, error) {
	logger := activity.GetLogger(ctx)
	
	// Marshal request body
	var bodyReader io.Reader
	if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			return HTTPCallResult{}, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}
	
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.Endpoint, bodyReader)
	if err != nil {
		return HTTPCallResult{}, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}
	
	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return HTTPCallResult{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HTTPCallResult{}, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Parse response body as JSON
	var responseData interface{}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &responseData); err != nil {
			// If JSON parsing fails, return as string
			responseData = string(respBody)
		}
	}
	
	logger.Info("HTTP call completed", "status", resp.StatusCode)
	return HTTPCallResult{
		Status: resp.StatusCode,
		Body:   responseData,
		Headers: func() map[string]string {
			headers := make(map[string]string)
			for key, values := range resp.Header {
				if len(values) > 0 {
					headers[key] = values[0]
				}
			}
			return headers
		}(),
	}, nil
}

// ExecuteBranchActivity executes a branch of tasks
func ExecuteBranchActivity(ctx context.Context, req ExecuteBranchRequest) (interface{}, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("ExecuteBranchActivity started", "tasks", len(req.Tasks))
	
	// Execute tasks sequentially within this branch
	branchState := make(map[string]interface{})
	var lastResult interface{}
	
	// Copy parent state to branch state
	for key, value := range req.State {
		branchState[key] = value
	}
	
	for i, taskItem := range req.Tasks {
		logger.Info("Executing branch task", "index", i, "key", taskItem.Key)
		
		// Execute each task based on its type
		var result interface{}
		var err error
		
		if doTask := taskItem.AsDoTask(); doTask != nil {
			// Handle "do" tasks which contain nested task lists
			result, err = executeBranchDoTask(ctx, doTask, branchState)
		} else if httpTask := taskItem.AsCallHTTPTask(); httpTask != nil {
			result, err = executeBranchHTTPTask(ctx, httpTask)
		} else if setTask := taskItem.AsSetTask(); setTask != nil {
			result, err = executeBranchSetTask(setTask, branchState)
		} else {
			err = fmt.Errorf("unsupported task type in branch: %s", taskItem.Key)
		}
		
		if err != nil {
			logger.Error("Branch task failed", "task", taskItem.Key, "error", err)
			return nil, fmt.Errorf("branch task %s failed: %w", taskItem.Key, err)
		}
		
		lastResult = result
		branchState[taskItem.Key] = result
	}
	
	logger.Info("Branch execution completed", "tasks", len(req.Tasks))
	return map[string]interface{}{
		"branch_result": lastResult,
		"task_count":    len(req.Tasks),
		"state":         branchState,
	}, nil
}

// executeBranchDoTask executes "do" tasks within a branch
func executeBranchDoTask(ctx context.Context, doTask *model.DoTask, state map[string]interface{}) (interface{}, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Executing branch do task", "nestedTasks", len(*doTask.Do))
	
	// Execute nested tasks sequentially
	var lastResult interface{}
	for i, nestedTaskItem := range *doTask.Do {
		logger.Info("Executing nested task in branch", "index", i, "key", nestedTaskItem.Key)
		
		var result interface{}
		var err error
		
		if httpTask := nestedTaskItem.AsCallHTTPTask(); httpTask != nil {
			result, err = executeBranchHTTPTask(ctx, httpTask)
		} else if setTask := nestedTaskItem.AsSetTask(); setTask != nil {
			result, err = executeBranchSetTask(setTask, state)
		} else {
			err = fmt.Errorf("unsupported nested task type in branch: %s", nestedTaskItem.Key)
		}
		
		if err != nil {
			return nil, fmt.Errorf("nested task %s failed: %w", nestedTaskItem.Key, err)
		}
		
		lastResult = result
		state[nestedTaskItem.Key] = result
	}
	
	return lastResult, nil
}

// executeBranchHTTPTask executes HTTP tasks within a branch
func executeBranchHTTPTask(ctx context.Context, httpTask *model.CallHTTP) (interface{}, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("Executing branch HTTP task", "endpoint", httpTask.With.Endpoint.String())
	
	// Use the same HTTP call logic as the main workflow
	req := HTTPCallRequest{
		Method:   httpTask.With.Method,
		Endpoint: httpTask.With.Endpoint.String(),
		Body:     httpTask.With.Body,
		Headers:  httpTask.With.Headers,
	}
	
	return executeRegularHTTPCall(ctx, req)
}

// executeBranchSetTask executes set tasks within a branch
func executeBranchSetTask(setTask *model.SetTask, state map[string]interface{}) (interface{}, error) {
	for key, value := range setTask.Set {
		state[key] = value
	}
	return setTask.Set, nil
}
