package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	port := "3001"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"server":    fmt.Sprintf("Backend Server on port %s", port),
			"timestamp": time.Now().Format(time.RFC3339),
			"method":    r.Method,
			"path":      r.URL.Path,
			"headers":   r.Header,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		users := []map[string]string{
			{"id": "1", "name": "Alice", "server": port},
			{"id": "2", "name": "Bob", "server": port},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	})

	log.Printf("Test backend server listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
