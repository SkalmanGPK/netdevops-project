package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	nodeName := os.Getenv("NODE_NAME")
	podIP := os.Getenv("POD_IP")
	fmt.Printf("Starting Mesh-Pinger v3 on Node: %s (IP: %s)\n", nodeName, podIP)

	// 1. Starta en enkel HTTP-server i bakgrunden som grannarna kan pinga
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "OK")
		})
		fmt.Println("Server listening on :8080...")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			fmt.Printf("HTTP Server failed: %v\n", err)
		}
	}()

	// 2. Konfigurera Kubernetes-klienten
	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Printf("Could not get config: %v\n", err)
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Could not create clientset: %v\n", err)
		os.Exit(1)
	}

	// Skapa en HTTP-klient med timeout (viktigt i nätverksprogrammering!)
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
	}

	// 3. Huvudloop för Service Discovery och Latensmätning
	for {
		// Hämta alla poddar med label app=mesh-pinger
		pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
			LabelSelector: "app=mesh-pinger",
		})
		if err != nil {
			fmt.Printf("Error listing pods: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}

		fmt.Printf("\n--- Mesh Check: %d pods found ---\n", len(pods.Items))

		for _, pod := range pods.Items {
			targetIP := pod.Status.PodIP

			// Pinga inte oss själva och hoppa över poddar utan IP
			if targetIP != "" && targetIP != podIP {
				targetNode := pod.Spec.NodeName
				
				// Mät latensen
				start := time.Now()
				url := fmt.Sprintf("http://%s:8080/health", targetIP)
				
				resp, err := httpClient.Get(url)
				if err != nil {
					fmt.Printf("FAILED to reach %s on %s: %v\n", targetIP, targetNode, err)
					continue
				}
				
				latency := time.Since(start)
				resp.Body.Close() // Stäng alltid body!

				fmt.Printf("LATENCY to %s (%s): %v (Status: %s)\n", 
					targetNode, targetIP, latency, resp.Status)
			}
		}

		// Vänta 5 sekunder innan nästa mätning
		time.Sleep(5 * time.Second)
	}
}
