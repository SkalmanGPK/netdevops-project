package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	// Load .env locally for development, ignore error if .env file is not present (e.g. in production)
	if err := loadEnv(); err != nil {
		fmt.Println("No .env file found, proceeding with environment variables")
	}
	// Creates a logger which writes structured JSON to stdout
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	nodeName := os.Getenv("NODE_NAME")
	podIP := os.Getenv("POD_IP")

	// Get target for which apps we ping from env variable, default to "app=mesh-pinger" if not set
	targetApp := os.Getenv("TARGET_LABEL")
	if targetApp == "" {
		targetApp = "mesh-pinger"
	}
	labelSelector := fmt.Sprintf("app=%s", targetApp)
	logger.Info("Starting Mesh-Pinger v3", "node", nodeName, "ip", podIP, "target_app", targetApp)

	// Starts a simple HTTP server in the background that neighbors can ping
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "OK")
		})
		logger.Info("Server listening on :8090...")
		if err := http.ListenAndServe(":8090", nil); err != nil {
			logger.Error("HTTP Server failed", "error", err)
		}
	}()

	// Utilize the service account token mounted inside the pod to authenticate against the k8s API.
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Could not get config", "error", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("Could not create clientset", "error", err)
		os.Exit(1)
	}

	// Creates a HTTP-client with timeout
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Main loop for Service Discovery and Latency measurement.
	for {

		// define where the main application is running
		localAppURL := "http://127.0.0.1:8081/health"

		// Check local application health
		localResp, err := httpClient.Get(localAppURL)
		if err != nil {
			logger.Error("Failed to reach local application", "url", localAppURL, "error", err)
			time.Sleep(5 * time.Second)
			continue // Skip the rest of the loop if local app is not healthy
		}

		// Control statuscode of local application health check
		if localResp.StatusCode != http.StatusOK {
			logger.Error("Local application is unhealthy", "url", localAppURL, "status", localResp.Status)
			localResp.Body.Close()
			time.Sleep(5 * time.Second)
			continue // Skip the rest of the loop if local app is not healthy
		}
		localResp.Body.Close() // Always close body

		// Query the K8s API server to discover all peer pods belonging to the mesh network.
		pods, err := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			logger.Error("Error listing pods", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}

		logger.Info("Mesh Check", "pod_count", len(pods.Items))

		for _, pod := range pods.Items {
			targetIP := pod.Status.PodIP

			// Avoid self-pinging and skip pods that haven't been assigned an IP yet by the CNI plugin.
			if targetIP != "" && targetIP != podIP {
				targetNode := pod.Spec.NodeName

				// Measure latency
				start := time.Now()
				url := fmt.Sprintf("http://%s:8090/health", targetIP)

				resp, err := httpClient.Get(url)
				if err != nil {
					logger.Error("Failed to reach pod", "ip", targetIP, "node", targetNode, "error", err)
					continue
				}

				latency := time.Since(start)
				resp.Body.Close() // Always close body

				logger.Info("LATENCY to pod", "node", targetNode, "ip", targetIP, "latency", latency, "status", resp.Status)
			}
		}

		// Wait 5 seconds before the next measurement
		time.Sleep(5 * time.Second)
	}
}
