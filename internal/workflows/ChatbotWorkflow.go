package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/serverlessworkflow/sdk-go/v3/parser"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
)

type ChatbotState struct {
	Conversation []anthropic.MessageParam `json:"conversation"`
	ThreadID     string                   `json:"thread_id"`
	SystemPrompt string                   `json:"system_prompt"`
	IsProcessing bool                     `json:"is_processing"`
}

type UserInputSignal struct {
	Message string `json:"message"`
}

type WorkflowValidationResult struct {
	HasWorkflow     bool   `json:"has_workflow"`
	IsValid         bool   `json:"is_valid"`
	ValidationError string `json:"validation_error"`
	WorkflowCode    string `json:"workflow_code"`
}

type ChatbotActivities struct {
	client *anthropic.Client
}

func NewChatbotActivities() *ChatbotActivities {
	apiKey := getClaudeAPIKey()
	if apiKey == "" {
		return &ChatbotActivities{client: nil}
	}
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &ChatbotActivities{
		client: &client,
	}
}

func ChatbotWorkflow(ctx workflow.Context, threadID string) (*ChatbotState, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("ChatbotWorkflow started", "threadID", threadID)

	// Initialize with system prompt for serverless workflow assistance
	systemPrompt := `You are a specialized assistant dedicated to helping users create workflow definitions in YAML based on the CNCF Serverless Workflow v1.0 specification. Your primary focus is to:

1. Help users design and create serverless workflow definitions in YAML format
2. Follow the official schema specification from https://serverlessworkflow.io/schemas/1.0.0/workflow.yaml
3. Ensure that all workflow definitions are valid and conform to the schema
4. all http calls should be made to sub-paths under localhost:8088/demo
5. do not use "uri", use "endpoint" for http calls
6. Provide clear and concise YAML workflow definitions without unnecessary explanations or comments
7. use dsl version 1.0.0, not 1.0.0-alpha1


Make sure to prioritize creating valid, well-structured workflow definitions that conform to the CNCF Serverless Workflow v1.0 specification with json spec at https://serverlessworkflow.io/schemas/1.0.0/workflow.json. Ask clarifying questions about the user's requirements to build the most appropriate workflow for their use case.`

	state := &ChatbotState{
		ThreadID:     threadID,
		Conversation: []anthropic.MessageParam{},
		SystemPrompt: systemPrompt,
		IsProcessing: false,
	}

	ao := workflow.ActivityOptions{
		StartToCloseTimeout: time.Second * 30,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	err := workflow.SetQueryHandler(ctx, "get-state", func() (*ChatbotState, error) {
		return state, nil
	})
	if err != nil {
		logger.Error("Failed to set query handler", "error", err)
		return nil, err
	}

	signalCh := workflow.GetSignalChannel(ctx, "user-input")

	for {
		var userInput UserInputSignal
		signalReceived, _ := signalCh.ReceiveWithTimeout(ctx, 10*time.Minute, &userInput)
		if !signalReceived {
			logger.Info("ChatbotWorkflow completed - timeout after 10 minutes of inactivity")
			break
		}

		logger.Info("Received user input", "message", userInput.Message)

		// Mark as processing when we start handling the user input
		state.IsProcessing = true
		state.Conversation = append(state.Conversation, anthropic.NewUserMessage(anthropic.NewTextBlock(userInput.Message)))

		activities := NewChatbotActivities()

		// Try to get a valid response, with retries for invalid workflows
		maxRetries := 4
		for retry := 0; retry < maxRetries; retry++ {
			var response *anthropic.Message
			err := workflow.ExecuteActivity(ctx, activities.CallClaudeAPI, state.SystemPrompt, state.Conversation).Get(ctx, &response)
			if err != nil {
				logger.Error("Failed to call Claude API", "error", err)

				errorResponse := "I apologize, but I'm having trouble connecting to the AI service right now. Please try again later."
				state.Conversation = append(state.Conversation, anthropic.NewAssistantMessage(anthropic.NewTextBlock(errorResponse)))
				state.IsProcessing = false
				break
			}

			// Check if the response contains a valid workflow (check YAML first since system prompt requests YAML)
			validationResult := validateWorkflowInResponse(response, "yaml")
			if validationResult.ValidationError != "" {
				logger.Error("Failed to validate workflow in response", "error", validationResult.ValidationError)
			}

			if validationResult.IsValid {
				// Valid workflow, add response and continue
				state.Conversation = append(state.Conversation, response.ToParam())
				logger.Info("Added Claude response to chat history", "response", response.Content)
				state.IsProcessing = false
				break
			} else if validationResult.HasWorkflow {
				// Invalid workflow, ask Claude to fix it
				logger.Info("Invalid workflow detected, asking Claude to fix it", "error", validationResult.ValidationError)

				// Add the response first
				state.Conversation = append(state.Conversation, response.ToParam())

				// Then add correction request
				correctionPrompt := fmt.Sprintf("The workflow you provided has validation errors:\n\n%s\n\nPlease correct the workflow and provide a valid YAML workflow definition.", validationResult.ValidationError)
				state.Conversation = append(state.Conversation, anthropic.NewUserMessage(anthropic.NewTextBlock(correctionPrompt)))

				// If this is the last retry, stop processing
				if retry == maxRetries-1 {
					state.IsProcessing = false
				}

				// Continue to next retry
				continue
			} else {
				// No workflow in response, just add it normally
				state.Conversation = append(state.Conversation, response.ToParam())
				logger.Info("Added Claude response to chat history", "response", response.Content)
				state.IsProcessing = false
				break
			}
		}
	}

	return state, nil
}

func (a *ChatbotActivities) CallClaudeAPI(ctx context.Context, systemPrompt string, conversation []anthropic.MessageParam) (*anthropic.Message, error) {
	logger := activity.GetLogger(ctx)
	logger.Info("CallClaudeAPI activity started")

	if a.client == nil {
		return nil, fmt.Errorf("claude API key not found in environment variables")
	}

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude4Sonnet20250514,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: conversation,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call Claude API: %w", err)
	}

	if len(resp.Content) == 0 {
		return nil, fmt.Errorf("empty response content from Claude API")
	}

	// Log if response was truncated due to max tokens
	if resp.StopReason == "max_tokens" {
		logger.Warn("Response was truncated due to max_tokens limit",
			"input_tokens", resp.Usage.InputTokens,
			"output_tokens", resp.Usage.OutputTokens)
	} else {
		logger.Info("Response completed normally",
			"stop_reason", resp.StopReason,
			"input_tokens", resp.Usage.InputTokens,
			"output_tokens", resp.Usage.OutputTokens)
	}

	return resp, nil
}

// validateWorkflowInResponse validates workflow code in Claude's response (deterministic function)
func validateWorkflowInResponse(response *anthropic.Message, format string) WorkflowValidationResult {
	result := WorkflowValidationResult{
		HasWorkflow:     false,
		IsValid:         false,
		ValidationError: "",
		WorkflowCode:    "",
	}

	// Extract text content from the response
	var responseText string
	if len(response.Content) > 0 {
		// Convert to JSON and extract text content
		contentBytes, err := json.Marshal(response.Content[0])
		if err == nil {
			var contentBlock map[string]interface{}
			if json.Unmarshal(contentBytes, &contentBlock) == nil {
				if text, exists := contentBlock["text"]; exists {
					if textStr, ok := text.(string); ok {
						responseText = textStr
					}
				}
			}
		}
	}

	if responseText == "" {
		return result
	}

	// Look for code blocks based on format
	var codeBlock string
	if format == "yaml" {
		codeBlock = extractYAMLCodeBlock(responseText)
		if codeBlock == "" {
			// Fallback to JSON if no YAML found
			codeBlock = extractJSONCodeBlock(responseText)
		}
	} else {
		codeBlock = extractJSONCodeBlock(responseText)
	}

	if codeBlock == "" {
		return result
	}

	result.HasWorkflow = true
	result.WorkflowCode = codeBlock

	// Try to parse the workflow based on format
	var err error
	if format == "yaml" {
		_, err = parser.FromYAMLSource([]byte(codeBlock))
	} else {
		_, err = parser.FromJSONSource([]byte(codeBlock))
	}

	if err != nil {
		result.IsValid = false
		result.ValidationError = err.Error()
	} else {
		result.IsValid = true
	}

	return result
}

// extractJSONCodeBlock extracts JSON code from markdown code blocks
func extractJSONCodeBlock(text string) string {
	// Look for ```json or ``` followed by JSON content
	patterns := []string{
		"```json\\s*\\n([\\s\\S]*?)```",
		"```\\s*\\n(\\{[\\s\\S]*?\\})```",
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// If no code block found, look for standalone JSON objects
	jsonPattern := `\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`
	re := regexp.MustCompile(jsonPattern)
	matches := re.FindAllString(text, -1)

	// Return the largest JSON object found
	var largestJSON string
	for _, match := range matches {
		if len(match) > len(largestJSON) {
			largestJSON = match
		}
	}

	return strings.TrimSpace(largestJSON)
}

// extractYAMLCodeBlock extracts YAML code from markdown code blocks
func extractYAMLCodeBlock(text string) string {
	// Look for ```yaml or ```yml followed by YAML content
	patterns := []string{
		"```yaml\\s*\\n([\\s\\S]*?)```",
		"```yml\\s*\\n([\\s\\S]*?)```",
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	return ""
}

func getClaudeAPIKey() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}
