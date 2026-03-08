package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create simple handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		message := fmt.Sprintf("Hello from test server! Time: %s", time.Now().Format(time.RFC3339))
		fmt.Fprintf(w, message)
	})
	// Health check endpoint created
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	addr := ":" + port
	log.Printf("Starting test server on %s", addr)
	log.Printf("Visit http://localhost%s/ to test", addr)
	log.Printf("Server PID: %d", os.Getpid())

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
