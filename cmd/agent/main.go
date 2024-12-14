package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	//"k8s.io/client-go/kubernetes"
	"html/template"
	robokubev1 "meliazeng/RoboKube/api/v1"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type QueueMessage struct {
	Sender   string    `json:"sender"`
	Message  string    `json:"message"`
	SentTime time.Time `json:"sentTime"`
}

// Global queue for messages
var messageQueue = struct {
	sync.RWMutex
	messages []QueueMessage
}{messages: make([]QueueMessage, 0)}

type SyncRequest struct {
	Target  string          `json:"target"`
	Message json.RawMessage `json:"message"`
}

var imagePath = filepath.Join("/data")

// Middleware to verify certificate
func certVerifyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		certHeader := r.Header.Get("X_FORWARDED_CLIENT_CERT")
		if certHeader != "" {
			//http.Error(w, "No certificate provided", http.StatusForbidden)
			//return

			block, _ := pem.Decode([]byte(certHeader))
			if block == nil {
				http.Error(w, "Failed to decode certificate", http.StatusForbidden)
				return
			}

			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				http.Error(w, "Invalid certificate", http.StatusForbidden)
				return
			}

			expectedCN := os.Getenv("EXPECTED_CN")
			if cert.Subject.CommonName != expectedCN {
				http.Error(w, "Invalid certificate common name", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Error retrieving image", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create images directory if it doesn't exist
	if err := os.MkdirAll(imagePath, 0755); err != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Create new file
	dst, err := os.Create(filepath.Join(imagePath, header.Filename))
	if err != nil {
		http.Error(w, "Error saving image", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Error saving image", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Read the full body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	// Close the original body
	r.Body.Close()
	// If decoding as a JSON object failed, reset the reader and try as a string
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Reset the body reader
	// Attempt to decode as a JSON object first
	var queueMsg QueueMessage
	decoder := json.NewDecoder(r.Body)  // Use a decoder for streaming parsing
	if err := decoder.Decode(&queueMsg); err == nil {
		// Successfully decoded as a JSON object
		messageQueue.Lock()
		messageQueue.messages = append(messageQueue.messages, queueMsg)
		messageQueue.Unlock()
		log.Printf("Message (JSON object) queued successfully: %+v", queueMsg)
		w.WriteHeader(http.StatusOK)
		return // Exit early; we've processed the message
	} else if err != io.EOF {  // Check if the error is NOT just an empty body
		log.Printf("Failed to decode message as JSON object: %v", err)
	}

	// If decoding as a JSON object failed, reset the reader and try as a string
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Reset the body reader
	var jsonString string
	if err := json.NewDecoder(r.Body).Decode(&jsonString); err != nil {
		log.Printf("Failed to decode message as JSON string: %v", err)
		http.Error(w, "Invalid message format (not JSON object or string)", http.StatusBadRequest)
		return
	}

	// Unmarshal the jsonString
	if err := json.Unmarshal([]byte(jsonString), &queueMsg); err != nil {
		log.Printf("Failed to unmarshal JSON string: %v", err)
		http.Error(w, "Invalid message content (invalid JSON string)", http.StatusBadRequest)
		return
	}
	/*
	// Read the full body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "Failed to read request", http.StatusBadRequest)
		return
	}
	// Close the original body
	r.Body.Close()

	// Create a new reader with the bytes for later use
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// Log the full body (for debugging)
	log.Printf("Request body: %s", string(bodyBytes))

    // First unmarshal the string-wrapped JSON
    var jsonString string
    if err := json.Unmarshal(bodyBytes, &jsonString); err != nil {
        log.Printf("Failed to decode JSON string: %v", err)
        http.Error(w, "Invalid message format", http.StatusBadRequest)
        return
    }

	var queueMsg QueueMessage
    if err := json.Unmarshal([]byte(jsonString), &queueMsg); err != nil {
        log.Printf("Failed to decode message content: %v", err)
        http.Error(w, "Invalid message content", http.StatusBadRequest)
        return
    }
    */
	messageQueue.Lock()
	messageQueue.messages = append(messageQueue.messages, queueMsg)
	messageQueue.Unlock()

	log.Printf("Message queued successfully from sender: %s at time: %v",
		queueMsg.Sender, queueMsg.SentTime)

	w.WriteHeader(http.StatusOK)
}

// Function to get current namespace
func getCurrentNamespace() string {
	// Try to get namespace from service account first
	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return string(data)
	}

	// Fallback to default namespace
	return "default"
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	// Get image count
	files, err := os.ReadDir(imagePath)
	if err != nil {
		log.Printf("Error reading images directory: %v", err)
		http.Error(w, "Error reading images directory", http.StatusInternalServerError)
		return
	}
	imageCount := 0
	for _, file := range files {
		if !file.IsDir() {
			imageCount++
		}
	}

	// Get message queue length
	messageQueue.RLock()
	messageCount := len(messageQueue.messages)
	messageQueue.RUnlock()

	// Create and send the JSON response
	status := struct {
		ImageCount   int `json:"imageCount"`
		MessageCount int `json:"messageCount"`
	}{
		ImageCount:   imageCount,
		MessageCount: messageCount,
	}
	log.Printf("Status: %+v", status)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func getEntityName() string {
	entityName := os.Getenv("ENTITY_NAME")
	if entityName == "" {
		log.Printf("Warning: ENTITY_NAME environment variable not set")
		return "unknown-entity"
	}

	// Remove the "-sa" suffix if it exists
	entityName = strings.TrimSuffix(entityName, "-sa")
	return entityName
}

type SyncResponse struct {
	TaskName string         `json:"taskName,omitempty"`
	Messages []QueueMessage `json:"messages"`
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := SyncResponse{
		Messages: make([]QueueMessage, 0),
	}

	if r.ContentLength != 0 {
		// Parse the incoming request
		var syncReq SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&syncReq); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		log.Printf("Target is: %s", syncReq.Target)

		if syncReq.Target != "" {
			// Create Kubernetes task
			sch := scheme.Scheme
			if err := robokubev1.AddToScheme(sch); err != nil { // Add scheme to scheme.Scheme
				http.Error(w, "Failed to add RoboKube scheme", http.StatusInternalServerError)
				return
			}

			config, err := rest.InClusterConfig()
			if err != nil {
				http.Error(w, "Kubernetes client error", http.StatusInternalServerError)
				return
			}

			kubeClient, err := client.New(config, client.Options{Scheme: sch})
			if err != nil {
				http.Error(w, "Failed to create Kubernetes client", http.StatusInternalServerError)
				return
			}

			queueMsg := QueueMessage{
				Sender:   getEntityName(),
				Message:  string(syncReq.Message),
				SentTime: time.Now(),
			}

			// Convert QueueMessage to JSON
			payloadBytes, err := json.Marshal(queueMsg)
			if err != nil {
				log.Printf("Failed to marshal queue message: %v", err)
				http.Error(w, "Failed to create task payload", http.StatusInternalServerError)
				return
			}

			// Create a new Task object
			task := &robokubev1.Task{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "sync-task-", // K8s will append a unique suffix
					Namespace:    getCurrentNamespace(),
				},
				Spec: robokubev1.TaskSpec{
					Description: "Task created via sync endpoint",
					Payload: apiextensionsv1.JSON{
							Raw: payloadBytes,
						},
					Targets: &[]robokubev1.TargetRef{
						{
							Type: robokubev1.TargetTypeEntity, // Or determine based on target format
							Name: syncReq.Target,
						},
					},
					// Set other fields as needed
					DryRun:          false,
					Persistant:      false,
					UnConditonalRun: false,
				},
			}

			// Create the task in Kubernetes
			ctx := context.Background()
			if err := kubeClient.Create(ctx, task); err != nil {
				http.Error(w, "Failed to create task: "+err.Error(), http.StatusInternalServerError)
				return
			}
			log.Printf("Successfully created task: %s", task.Name)
		}

	} else {
		// Get all messages from the queue
		messageQueue.Lock()
		if len(messageQueue.messages) > 0 {
			response.Messages = append(response.Messages, messageQueue.messages...)
			messageQueue.messages = make([]QueueMessage, 0) // Clear the queue
			log.Printf("Retrieved %d messages from queue", len(response.Messages))
		}
		messageQueue.Unlock()
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
	log.Printf("Sync request completed successfully")
}

const imagePage = `
<!DOCTYPE html>
<html>
<head>
    <title>Received Images</title>
</head>
<body>
    <h1>Received Images</h1>
    <div>
        {{range .}}
        <div>
            <img src="/images/{{.}}" alt="{{.}}" style="max-width: 300px">
            <p>{{.}}</p>
        </div>
        {{end}}
    </div>
</body>
</html>
`

func handleImageList(w http.ResponseWriter, r *http.Request) {
	files, err := os.ReadDir(imagePath)
	if err != nil {
		http.Error(w, "Error reading images", http.StatusInternalServerError)
		return
	}

	var imageFiles []string
	for _, file := range files {
		if !file.IsDir() {
			imageFiles = append(imageFiles, file.Name())
		}
	}

	tmpl := template.Must(template.New("images").Parse(imagePage))
	tmpl.Execute(w, imageFiles)
}

func main() {

	// Create a new mux router
	mux := http.NewServeMux()

	// Add routes with certificate verification middleware
	mux.Handle("/image", certVerifyMiddleware(http.HandlerFunc(handleImage)))
	mux.Handle("/sync", certVerifyMiddleware(http.HandlerFunc(handleSync)))
	mux.Handle("/task", http.HandlerFunc(handleTask))
	mux.Handle("/status", http.HandlerFunc(handleStatus))

	// Serve images directory with certificate verification
	mux.Handle("/images/", certVerifyMiddleware(
		http.StripPrefix("/images/", http.FileServer(http.Dir(imagePath))),
	))
	// Image list page also needs certificate verification
	mux.Handle("/", certVerifyMiddleware(http.HandlerFunc(handleImageList)))

	// Start the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server starting on port %s...\n", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Printf("Server error: %v\n", err)
		os.Exit(1)
	}
}
