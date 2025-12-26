package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/misleb/nodeport-router/controller"
	"github.com/misleb/nodeport-router/router"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	_ = godotenv.Load(".env")
	deviceName, ok := os.LookupEnv("K8S_HOST")
	if !ok {
		log.Fatalf("K8S_HOST is not set")
	}
	baseURL, ok := os.LookupEnv("ROUTER_BASE")
	if !ok {
		log.Fatalf("ROUTER_BASE is not set")
	}
	routerAdmin, ok := os.LookupEnv("ROUTER_ADMIN")
	if !ok {
		log.Fatalf("ROUTER_ADMIN is not set")
	}
	routerPass, ok := os.LookupEnv("ROUTER_PASS")
	if !ok {
		log.Fatalf("ROUTER_PASS is not set")
	}

	// Initialize Kubernetes client (works in-cluster or from kubeconfig)
	config, err := rest.InClusterConfig()
	if err != nil {
		// Fallback to kubeconfig if not in cluster
		config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		if err != nil {
			log.Fatalf("Error building kubeconfig: %v", err)
		}
	}

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Error creating Kubernetes client: %v", err)
	}

	routerClient := router.NewRouterClient(baseURL, routerAdmin, routerPass)
	log.Println("Authenticating to", baseURL)
	if err := routerClient.Login(); err != nil {
		log.Fatalf("Error logging in to router: %v", err)
	}

	controller := controller.Controller{
		K8sClient:    k8sClient,
		RouterClient: routerClient,
		DeviceName:   deviceName,
	}

	// Start watching Services
	ctx := context.Background()
	if err := controller.Run(ctx); err != nil {
		log.Fatalf("Error running controller: %v", err)
	}
}
