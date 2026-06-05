package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Config holds all environment-driven settings for the application.
type Config struct {
	LocalAppURL  string
	TargetLabel  string
	ListenAddr   string
	LoopInterval time.Duration
	HTTPTimeout  time.Duration
}

// loadConfig initializes configuration from environment variables with safe production defaults.
func loadConfig() Config {
	localPort := os.Getenv("LOCAL_APP_PORT")
	if localPort == "" {
		localPort = "8081"
	}

	intervalStr := os.Getenv("LOOP_INTERVAL")
	interval := 5 * time.Second
	if parsed, err := time.ParseDuration(intervalStr); err == nil {
		interval = parsed
	}

	targetApp := os.Getenv("TARGET_LABEL")
	if targetApp == "" {
		targetApp = "mesh-pinger"
	}

	return Config{
		LocalAppURL:  fmt.Sprintf("http://127.0.0.1:%s/health", localPort),
		TargetLabel:  targetApp,
		ListenAddr:   ":8090",
		LoopInterval: interval,
		HTTPTimeout:  2 * time.Second,
	}
}

// retryOp defines the function signature for operations that can return an error and need retries.
type retryOp func() error

// executeWithBackoff runs an operation and retries it using exponential backoff and jitter if it fails.
// It respects context cancellation and aborts immediately if a termination signal is received.
func executeWithBackoff(ctx context.Context, logger *slog.Logger, opName string, operation retryOp) error {
	currentWait := 1 * time.Second
	factor := 2.0
	maxWait := 30 * time.Second
	maxAttempts := 3

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Execute the network or API operation
		err := operation()
		if err == nil {
			return nil // Success
		}

		logger.Warn("Transient operation failed, initiating backoff retry",
			"operation", opName,
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"error", err.Error(),
		)

		if attempt == maxAttempts {
			return fmt.Errorf("%s exhausted all retry attempts: %w", opName, err)
		}

		// Calculate next exponential delay backoff
		nextWait := time.Duration(float64(currentWait) * factor)
		if nextWait > maxWait {
			nextWait = maxWait
		}

		// Apply Jitter (10% randomized noise) to mitigate the Thundering Herd problem
		// rand.Float64() generates a value between 0.0 and 1.0
		jitter := time.Duration((rand.Float64() - 0.5) * 0.1 * float64(nextWait))
		totalWait := nextWait + jitter

		logger.Info("Throttling before next retry attempt", "operation", opName, "backoff_duration", totalWait)

		// Wait interruptibly - handle mid-sleep pod termination gracefully
		select {
		case <-time.After(totalWait):
			currentWait = nextWait // Scale the base duration up for the next loop
		case <-ctx.Done():
			logger.Info("Backoff execution aborted due to context cancellation", "operation", opName)
			return ctx.Err()
		}
	}

	return fmt.Errorf("operation %s failed critically", opName)
}

func main() {
	// Load localized environment variables for development (defined in env.go)
	if err := loadEnv(); err != nil {
		fmt.Println("No local .env file discovered, utilizing standard runtime environment variables")
	}

	// Initialize structured JSON logging for standard output streaming
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg := loadConfig()

	nodeName := os.Getenv("NODE_NAME")
	podIP := os.Getenv("POD_IP")
	labelSelector := fmt.Sprintf("app=%s", cfg.TargetLabel)

	logger.Info("Starting State-of-the-Art Mesh-Pinger v3", "node", nodeName, "ip", podIP, "target_app", cfg.TargetLabel)

	// Intercept OS termination signals to coordinate a graceful microservice shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Configure the internal routing matrix and HTTP server architecture
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})
	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	// Launch background HTTP server to handle health probing requests from mesh neighbors
	go func() {
		logger.Info("Telemetry and probing server online", "listen_address", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Inbound HTTP Server crashed unexpectedly", "error", err)
		}
	}()

	// Establish authenticated access token connection to the local Kubernetes API control plane
	k8sConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Failed to resolve in-cluster RBAC configuration credentials", "error", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		logger.Error("Failed to instantiate client-go client interface connection", "error", err)
		os.Exit(1)
	}

	httpClient := &http.Client{
		Timeout: cfg.HTTPTimeout,
	}

	// Central Execution Loop - Drives service discovery and latency probing cycles
	for {
		select {
		case <-ctx.Done():
			// Triggered when Kubernetes issues a SIGTERM signal to decommission the container
			logger.Info("SIGTERM intercepted. Initiating graceful shutdown sequences...")

			// Allocate a hard maximum of 5 seconds to drain active networking requests
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := srv.Shutdown(shutdownCtx); err != nil {
				logger.Error("Inbound HTTP server failed to close cleanly during shutdown drain", "error", err)
			}

			logger.Info("Microservice resources cleaned up safely. System termination complete.")
			return

		default:
			// 1. Verify availability of the co-located main business application
			err := executeWithBackoff(ctx, logger, "LocalAppHealthProbe", func() error {
				localResp, err := httpClient.Get(cfg.LocalAppURL)
				if err != nil {
					return err
				}
				defer localResp.Body.Close()

				if localResp.StatusCode != http.StatusOK {
					return fmt.Errorf("unhealthy status returned: %s", localResp.Status)
				}
				return nil
			})

			if err != nil {
				logger.Error("Critical: Co-located application remains unreachable. Aborting current mesh sweep.", "error", err.Error())
				
				// Settle safely before evaluating the next scheduling cycle
				select {
				case <-time.After(cfg.LoopInterval):
				case <-ctx.Done():
				}
				continue
			}

			// 2. Query the Kubernetes API Server to dynamically discover endpoints matching the target mesh label
			var pods *metav1.PodList
			err = executeWithBackoff(ctx, logger, "KubernetesPodDiscovery", func() error {
				var apiErr error
				pods, apiErr = clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
					LabelSelector: labelSelector,
				})
				return apiErr
			})

			if err != nil {
				logger.Error("Critical: Target endpoint resolution failed via Kubernetes API", "error", err.Error())
				select {
				case <-time.After(cfg.LoopInterval):
				case <-ctx.Done():
				}
				continue
			}

			logger.Info("Mesh network discovery topology mapped", "active_mesh_pod_count", len(pods.Items))

			// 3. Iterate through all resolved network endpoints to record network latency measurements
			for _, pod := range pods.Items {
				targetIP := pod.Status.PodIP

				// Isolate tracking vectors: Exclude self-probing and unassigned endpoints
				if targetIP != "" && targetIP != podIP {
					targetNode := pod.Spec.NodeName

					start := time.Now()
					url := fmt.Sprintf("http://%s:8090/health", targetIP)

					resp, err := httpClient.Get(url)
					if err != nil {
						logger.Error("Failed to probe target mesh neighbor pod", "target_ip", targetIP, "target_node", targetNode, "error", err.Error())
						continue
					}

					latency := time.Since(start)
					resp.Body.Close()

					logger.Info("Mesh Network Telemetry Sample",
						"destination_node", targetNode,
						"destination_ip", targetIP,
						"rtt_latency", latency.String(),
						"http_status_code", resp.StatusCode,
					)
				}
			}

			// Safe throttle pause mechanism preventing rapid continuous cycling before initiating the next discovery sweep
			select {
			case <-time.After(cfg.LoopInterval):
			case <-ctx.Done():
			}
		}
	}
}