package controller

import (
	"context"
	"fmt"
	"log"

	"github.com/misleb/nodeport-router/router"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type Controller struct {
	K8sClient    kubernetes.Interface
	RouterClient *router.RouterClient
	DeviceName   string // e.g., "bow0"
}

func (c *Controller) Run(ctx context.Context) error {
	// Watch all Services across all namespaces (or filter to specific namespace)
	watcher, err := c.K8sClient.CoreV1().Services("").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error creating watcher: %v", err)
	}
	defer watcher.Stop()

	log.Println("Started watching Services for NodePort changes...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watcher channel closed")
			}

			service := event.Object.(*corev1.Service)

			// Only process NodePort services
			if service.Spec.Type != corev1.ServiceTypeNodePort {
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				if err := c.handleServiceSync(service); err != nil {
					log.Printf("Error syncing service %s/%s: %v", service.Namespace, service.Name, err)
				}
			case watch.Deleted:
				if err := c.handleServiceDelete(service); err != nil {
					log.Printf("Error deleting service %s/%s: %v", service.Namespace, service.Name, err)
				}
			}
		}
	}
}

func (c *Controller) handleServiceSync(service *corev1.Service) error {
	// Extract NodePort(s) from service
	for _, port := range service.Spec.Ports {
		if port.NodePort == 0 {
			continue
		}

		// Map NodePort to internal port (could be port.TargetPort.IntVal or port.Port)
		targetPort := port.Port
		if port.TargetPort.IntVal != 0 {
			targetPort = port.TargetPort.IntVal
		}

		forward := router.Forward{
			DeviceName:  c.DeviceName,
			ServiceName: fmt.Sprintf("%s-%s-%d", service.Namespace, service.Name, port.Port),
			Ports:       fmt.Sprintf("%d", targetPort),    // Internal port
			DevicePort:  fmt.Sprintf("%d", port.NodePort), // External port (NodePort)
		}

		// Get current forwards and check if this already exists
		// If exists with different ports, update it
		// If not exists, add it
		if err := c.syncForward(forward); err != nil {
			return fmt.Errorf("error syncing forward for service %s: %v", service.Name, err)
		}

		log.Printf("Synced NodePort %d -> %d for service %s/%s",
			port.NodePort, targetPort, service.Namespace, service.Name)
	}

	return nil
}

func (c *Controller) handleServiceDelete(service *corev1.Service) error {
	// Remove port forwards associated with this service
	// You'd need a deleteForward function similar to addForward
	log.Printf("Service %s/%s deleted, removing port forwards", service.Namespace, service.Name)
	return nil
}

func (c *Controller) syncForward(forward router.Forward) error {
	// Ensure router session is valid (login if needed)
	if err := c.RouterClient.EnsureLoggedIn(); err != nil {
		return err
	}

	// Get current forwards to check if update is needed
	forwards := []router.Forward{}
	_, err := c.RouterClient.GetForwards(&forwards)
	if err != nil {
		return err
	}

	// Check if forward exists
	exists := false
	for _, f := range forwards {
		if f.ServiceName == forward.ServiceName {
			exists = true
			// If ports changed, update it (you'd need an updateForward function)
			if f.Ports != forward.Ports || f.DevicePort != forward.DevicePort {
				// Update logic here
			}
			break
		}
	}

	if !exists {
		return c.RouterClient.AddForward(forward)
	}

	return nil
}
