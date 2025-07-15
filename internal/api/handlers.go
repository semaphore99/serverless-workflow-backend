package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/semaphore99/serverless-workflow-backend/internal/workflows"
	"go.temporal.io/sdk/client"
)

type Handlers struct {
	temporal client.Client
}

func New(temporalClient client.Client) *Handlers {
	return &Handlers{temporal: temporalClient}
}

func (h *Handlers) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"healthy": true})
}

func (h *Handlers) ExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Read the raw JSON payload
	workflowJSONBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	options := client.StartWorkflowOptions{
		ID:        "serverless-workflow-" + uuid.New().String(),
		TaskQueue: "serverless-workflow-task-queue",
	}

	wfRun, err := h.temporal.ExecuteWorkflow(r.Context(), options, workflows.ExecuteServerlessYAMLWorkflow, string(workflowJSONBytes))
	if err != nil {
		log.Printf("Unable to execute workflow: %v", err)
		http.Error(w, "Failed to execute workflow", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"workflow_id": wfRun.GetID()})
}

func (h *Handlers) ExecuteJSONWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	// Read the raw JSON payload
	workflowJSONBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	options := client.StartWorkflowOptions{
		ID:        "json-workflow-" + uuid.New().String(),
		TaskQueue: "serverless-workflow-task-queue",
	}

	wfRun, err := h.temporal.ExecuteWorkflow(r.Context(), options, workflows.ExecuteServerlessJSONWorkflow, string(workflowJSONBytes))
	if err != nil {
		log.Printf("Unable to execute JSON workflow: %v", err)
		http.Error(w, "Failed to execute JSON workflow", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"workflow_id": wfRun.GetID()})
}

func (h *Handlers) InitiateChatbot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	threadID := uuid.New().String()

	options := client.StartWorkflowOptions{
		ID:        "chatbot-workflow-" + threadID,
		TaskQueue: "serverless-workflow-task-queue",
	}

	wfRun, err := h.temporal.ExecuteWorkflow(r.Context(), options, workflows.ChatbotWorkflow, threadID)
	if err != nil {
		log.Printf("Unable to initiate chatbot workflow: %v", err)
		http.Error(w, "Failed to initiate chatbot workflow", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"workflow_id": wfRun.GetID(),
		"thread_id":   threadID,
	})
}

func (h *Handlers) SendChatMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	var requestBody struct {
		ThreadID string `json:"thread_id"`
		Message  string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if requestBody.ThreadID == "" || requestBody.Message == "" {
		http.Error(w, "thread_id and message are required", http.StatusBadRequest)
		return
	}

	workflowID := "chatbot-workflow-" + requestBody.ThreadID

	// Get initial conversation length
	var initialState *workflows.ChatbotState
	resp, err := h.temporal.QueryWorkflow(r.Context(), workflowID, "", "get-state")
	if err != nil {
		log.Printf("Unable to query initial workflow state: %v", err)
		http.Error(w, "Failed to get workflow state", http.StatusInternalServerError)
		return
	}
	err = resp.Get(&initialState)
	if err != nil {
		log.Printf("Unable to decode initial query result: %v", err)
		http.Error(w, "Failed to decode workflow state", http.StatusInternalServerError)
		return
	}
	initialLength := len(initialState.Conversation)

	// Send user input signal
	err = h.temporal.SignalWorkflow(r.Context(), workflowID, "", "user-input", workflows.UserInputSignal{
		Message: requestBody.Message,
	})
	if err != nil {
		log.Printf("Unable to signal workflow: %v", err)
		http.Error(w, "Failed to send message", http.StatusInternalServerError)
		return
	}

	// Poll for the response (wait for conversation to grow by 2: user message + assistant response)
	for i := 0; i < 30; i++ { // Max 30 attempts (30 seconds)
		var currentState *workflows.ChatbotState
		resp, err := h.temporal.QueryWorkflow(r.Context(), workflowID, "", "get-state")
		if err != nil {
			log.Printf("Unable to query workflow state: %v", err)
			http.Error(w, "Failed to get workflow state", http.StatusInternalServerError)
			return
		}
		err = resp.Get(&currentState)
		if err != nil {
			log.Printf("Unable to decode query result: %v", err)
			http.Error(w, "Failed to decode workflow state", http.StatusInternalServerError)
			return
		}

		// Check if we have the assistant's response (conversation should have grown by 2) AND processing is complete
		if len(currentState.Conversation) >= initialLength+2 && !currentState.IsProcessing {
			lastMessage := currentState.Conversation[len(currentState.Conversation)-1]

			// Extract text content from the assistant's response
			var responseText string
			if lastMessage.Role == "assistant" && len(lastMessage.Content) > 0 {
				// Convert to JSON and back to extract text content
				contentBytes, err := json.Marshal(lastMessage.Content[0])
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

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":   true,
				"response":  responseText,
				"thread_id": requestBody.ThreadID,
			})
			return
		}

		// Wait 1 second before polling again
		time.Sleep(1 * time.Second)
	}

	// Timeout - return success but indicate we couldn't get the response in time
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"response":  "Processing your message...",
		"thread_id": requestBody.ThreadID,
		"note":      "Response is being processed. Use GET /workflow/thread to retrieve the full conversation.",
	})
}

func (h *Handlers) GetChatThread(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}

	threadID := r.URL.Query().Get("thread_id")
	if threadID == "" {
		http.Error(w, "thread_id parameter is required", http.StatusBadRequest)
		return
	}

	workflowID := "chatbot-workflow-" + threadID

	var result *workflows.ChatbotState
	resp, err := h.temporal.QueryWorkflow(r.Context(), workflowID, "", "get-state")
	if err != nil {
		log.Printf("Unable to query workflow: %v", err)
		http.Error(w, "Failed to fetch chat thread", http.StatusInternalServerError)
		return
	}

	err = resp.Get(&result)
	if err != nil {
		log.Printf("Unable to decode query result: %v", err)
		http.Error(w, "Failed to decode chat thread state", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
