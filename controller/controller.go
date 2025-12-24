package controller

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/misleb/nodeport-router/router"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type Controller struct {
	K8sClient    kubernetes.Interface
	RouterClient *router.RouterClient
	DeviceName   string // e.g., "bow0"
	informer     cache.SharedInformer
}

func (c *Controller) Run(ctx context.Context) error {
	// Create an informer factory
	// Use "" for all namespaces, or specify a namespace
	informerFactory := informers.NewSharedInformerFactory(c.K8sClient, time.Second*30)

	// Create a Service informer
	serviceInformer := informerFactory.Core().V1().Services()

	// Set up event handlers
	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			service := obj.(*corev1.Service)
			if service.Spec.Type != corev1.ServiceTypeNodePort {
				return
			}
			if err := c.handleServiceAdd(service); err != nil {
				log.Printf("Error syncing service %s/%s: %v", service.Namespace, service.Name, err)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldService := oldObj.(*corev1.Service)
			newService := newObj.(*corev1.Service)

			// Only process NodePort services
			if newService.Spec.Type != corev1.ServiceTypeNodePort {
				return
			}

			// Check if NodePort values actually changed
			if c.nodePortsChanged(oldService, newService) {
				log.Printf("NodePort changed for service %s/%s", newService.Namespace, newService.Name)
				if err := c.handleServiceUpdate(oldService, newService); err != nil {
					log.Printf("Error updating service %s/%s: %v", newService.Namespace, newService.Name, err)
				}
			} else {
				log.Printf("NodePort values did not change for service %s/%s", newService.Namespace, newService.Name)
			}
		},
		DeleteFunc: func(obj interface{}) {
			service := obj.(*corev1.Service)
			if service.Spec.Type != corev1.ServiceTypeNodePort {
				return
			}
			if err := c.handleServiceDelete(service); err != nil {
				log.Printf("Error deleting service %s/%s: %v", service.Namespace, service.Name, err)
			}
		},
	})

	c.informer = serviceInformer.Informer()

	// Start the informer
	log.Println("Starting informer...")
	informerFactory.Start(ctx.Done())

	// Wait for the cache to sync
	log.Println("Waiting for cache to sync...")
	if !cache.WaitForCacheSync(ctx.Done(), c.informer.HasSynced) {
		return fmt.Errorf("failed to sync cache")
	}

	log.Println("Started watching Services for NodePort changes...")

	// Block until context is cancelled
	<-ctx.Done()
	return ctx.Err()
}

// nodePortsChanged checks if NodePort values changed between old and new service
func (c *Controller) nodePortsChanged(oldService, newService *corev1.Service) bool {
	oldPorts := make(map[string]string) // port name -> nodePort+port
	newPorts := make(map[string]string) // port name -> nodePort+port

	for _, port := range oldService.Spec.Ports {
		if port.NodePort != 0 {
			oldPorts[port.Name] = fmt.Sprintf("%d-%d", port.NodePort, port.Port)
		}
	}

	for _, port := range newService.Spec.Ports {
		if port.NodePort != 0 {
			newPorts[port.Name] = fmt.Sprintf("%d-%d", port.NodePort, port.Port)
		}
	}

	// Check if maps are different
	return !equality.Semantic.DeepEqual(oldPorts, newPorts)
}

// handleServiceUpdate handles updates where NodePort values changed
func (c *Controller) handleServiceUpdate(oldService, newService *corev1.Service) error {
	// Extract old and new forwards
	oldForwards := c.affectedForwards(oldService)
	newForwards := c.affectedForwards(newService)

	for _, forward := range oldForwards {
		if err := c.RouterClient.DeleteForward(forward); err != nil {
			// Not a fatal error, just log it.
			log.Printf("error deleting forward for service %s: %v", oldService.Name, err)
		}
	}

	for _, forward := range newForwards {
		if err := c.RouterClient.AddForward(forward); err != nil {
			return fmt.Errorf("error adding forward for service %s: %v", newService.Name, err)
		}

		log.Printf("Updated NodePort %s -> %s for service %s/%s",
			forward.DevicePort, forward.Ports, newService.Namespace, newService.Name)
	}

	return nil
}

func (c *Controller) handleServiceAdd(service *corev1.Service) error {
	// Extract NodePort(s) from service
	forwards := c.affectedForwards(service)

	for _, forward := range forwards {
		if err := c.RouterClient.AddForward(forward); err != nil {
			return fmt.Errorf("error syncing forward for service %s: %v", service.Name, err)
		}

		log.Printf("Added NodePort %s -> %s for service %s/%s",
			forward.DevicePort, forward.Ports, service.Namespace, service.Name)
	}

	return nil
}

func (c *Controller) handleServiceDelete(service *corev1.Service) error {
	log.Printf("Service %s/%s deleted, removing port forwards", service.Namespace, service.Name)
	forwards := c.affectedForwards(service)
	for _, forward := range forwards {
		if err := c.RouterClient.DeleteForward(forward); err != nil {
			return fmt.Errorf("error deleting forward for service %s: %v", service.Name, err)
		}
		log.Printf("Removed NodePort %s -> %s for service %s/%s",
			forward.DevicePort, forward.Ports, service.Namespace, service.Name)
	}
	return nil
}

func (c *Controller) affectedForwards(service *corev1.Service) []router.Forward {
	forwards := []router.Forward{}
	for _, port := range service.Spec.Ports {
		if port.NodePort == 0 {
			continue
		}
		forwards = append(forwards, router.Forward{
			DeviceName:  c.DeviceName,
			Ports:       fmt.Sprintf("%d", port.Port),
			DevicePort:  fmt.Sprintf("%d", port.NodePort),
			ServiceName: fmt.Sprintf("%s-%s-%d-%d", service.Namespace, service.Name, port.Port, port.NodePort),
		})
	}
	return forwards
}
