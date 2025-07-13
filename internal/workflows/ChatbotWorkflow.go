package workflows

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
)

type ChatbotState struct {
	Conversation []anthropic.MessageParam `json:"conversation"`
	ThreadID     string                   `json:"thread_id"`
	SystemPrompt string                   `json:"system_prompt"`
}

type UserInputSignal struct {
	Message string `json:"message"`
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
	systemPrompt := `You are a specialized assistant dedicated to helping users create workflow definitions based on the CNCF Serverless Workflow v1.0 specification. Your primary focus is to:

1. Help users design and create serverless workflow definitions in JSON format
2. Follow the official schema specification from https://serverlessworkflow.io/schemas/1.0.0/workflow.json
3. Ensure that all workflow definitions are valid and conform to the schema

When assisting users, always prioritize creating valid, well-structured workflow definitions that conform to the CNCF Serverless Workflow v1.0 specification. Ask clarifying questions about the user's requirements to build the most appropriate workflow for their use case.`

	state := &ChatbotState{
		ThreadID:     threadID,
		Conversation: []anthropic.MessageParam{},
		SystemPrompt: systemPrompt,
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

		state.Conversation = append(state.Conversation, anthropic.NewUserMessage(anthropic.NewTextBlock(userInput.Message)))

		activities := NewChatbotActivities()
		var response *anthropic.Message
		err := workflow.ExecuteActivity(ctx, activities.CallClaudeAPI, state.SystemPrompt, state.Conversation).Get(ctx, &response)
		if err != nil {
			logger.Error("Failed to call Claude API", "error", err)

			errorResponse := "I apologize, but I'm having trouble connecting to the AI service right now. Please try again later."
			state.Conversation = append(state.Conversation, anthropic.NewAssistantMessage(anthropic.NewTextBlock(errorResponse)))
			continue
		}

		state.Conversation = append(state.Conversation, response.ToParam())

		logger.Info("Added Claude response to chat history", "response", response.Content)
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

func getClaudeAPIKey() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}
