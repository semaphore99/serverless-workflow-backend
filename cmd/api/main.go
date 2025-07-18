package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/semaphore99/serverless-workflow-backend/internal/api"
	"github.com/semaphore99/serverless-workflow-backend/internal/workflows"
	"go.temporal.io/sdk/client"
)

func main() {
	temporalClient, err := client.Dial(client.Options{})
	if err != nil {
		log.Fatalln("Unable to create Temporal client", err)
	}
	defer temporalClient.Close()

	worker := workflows.StartWorker(temporalClient)
	go func() {
		err := worker.Run(nil)
		if err != nil {
			log.Fatalln("Unable to start worker", err)
		}
	}()

	handlers := api.New(temporalClient)

	http.HandleFunc("/health", handlers.HealthCheck)
	http.HandleFunc("/workflows", handlers.ExecuteWorkflow)
	http.HandleFunc("/workflows/json", handlers.ExecuteJSONWorkflow)
	http.HandleFunc("/workflows/yaml", handlers.ExecuteYAMLWorkflow)
	http.HandleFunc("/workflows/state", handlers.GetWorkflowState)
	http.HandleFunc("/chatbot/init", handlers.InitiateChatbot)
	http.HandleFunc("/chatbot/message", handlers.SendChatMessage)
	http.HandleFunc("/chatbot/thread", handlers.GetChatThread)
	http.HandleFunc("/demo/", handlers.DemoHandler)

	server := &http.Server{
		Addr: ":8088",
	}

	go func() {
		log.Println("Starting server on :8088")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down...")

	// Stop the worker first
	worker.Stop()
	log.Println("Worker stopped")

	// Then shutdown the HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
