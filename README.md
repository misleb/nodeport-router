# nodeport-router

A Kubernetes controller that automatically configures port forwarding rules on your router when NodePort services are created or modified in your cluster. This allows external access to your Kubernetes services through your router's port forwarding configuration.

## Overview

`nodeport-router` watches for Kubernetes NodePort services and automatically manages port forwarding rules on supported routers. When a NodePort service is created or updated, the controller adds or updates the corresponding port forward rule on your router, making your services accessible from outside your network.

## Supported Routers

- **Arris NVG443B** (tested and supported)

## Features

- 🔄 Automatic port forward management based on Kubernetes NodePort services
- 👀 Watches all namespaces for NodePort service changes
- 🔐 Secure authentication with router web interface
- 🚀 Runs as a Kubernetes deployment
- 🔒 Non-root container execution with minimal RBAC permissions

## Architecture

The controller consists of three main components:

1. **Controller** (`controller/controller.go`): Watches Kubernetes services and manages the sync process
2. **Router Client** (`router/router.go`): Handles communication with the router's web interface
3. **Main** (`main.go`): Initializes clients and starts the controller

## Prerequisites

- Kubernetes cluster (Tested on a fairly stock Talos Linux v1.11.6 cluster)
- Router with web interface (Arris NVG443B or compatible)
- Network access from the Kubernetes cluster to the router
- `kubectl` configured to access your cluster

## Configuration

The controller requires the following environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `K8S_HOST` | Device name of your cluster entrypoint on the router | `talos0` |
| `ROUTER_BASE` | Base URL of the router web interface | `http://192.168.254.254` |
| `ROUTER_ADMIN` | Router admin username | `admin` |
| `ROUTER_PASS` | Router admin password | `password` |

## Installation

### 1. Build the Docker Image (Optional if you don't want to use my Docker Hub image)

```bash
docker build -t your-registry/nodeport-router:latest .
docker push your-registry/nodeport-router:latest
```

### 2. Update Deployment Configuration

Edit `deployment.yaml` and update the following non-sensitive variables:

- **ConfigMap values**:
  - `K8S_HOST`: Your cluster entrypoint as known by the router
  - `ROUTER_BASE`: Your router's internal web interface URL

- **Container image**: Update the image name if using a different registry

### 3. Create Kubernetes Secret

Create a secret with your router credentials:

```bash
kubectl create secret generic nodeport-router-secrets \
  --from-literal=ROUTER_ADMIN='your-router-username' \
  --from-literal=ROUTER_PASS='your-router-password' \
  --namespace=default
```

### 4. Deploy to Kubernetes

```bash
kubectl apply -f deployment.yaml
```

### 5. Verify Deployment

Check that the pod is running:

```bash
kubectl get pods -l app=nodeport-router
kubectl logs -l app=nodeport-router
```

## How It Works

1. The controller watches all Kubernetes services across all namespaces
2. When a NodePort service is detected (created or modified):
   - Extracts the NodePort and target port from the service
   - Creates a port forward rule on the router mapping:
     - External port: NodePort
     - Internal port: Target port
     - Device: Configured `K8S_HOST`
3. When a NodePort service is deleted, the corresponding port forward rules are removed (TODO: deletion logic needs to be implemented)

## Port Forward Naming

Port forwards are named using the pattern:
```
{namespace}-{service-name}-{port}
```

For example, a service named `myapp` in the `default` namespace with port `8080` will create a forward named `default-myapp-8080`.

## Local Development

### Prerequisites

- Go 1.25.1 or later
- Access to a Kubernetes cluster (via kubeconfig or in-cluster)
- Network access to your router

### Setup

1. Clone the repository:
```bash
git clone https://github.com/misleb/nodeport-router.git
cd nodeport-router
```

2. Install dependencies:
```bash
go mod download
```

3. Create a `.env` file with your configuration:
```env
K8S_HOST=bow0
ROUTER_BASE=http://192.168.254.254
ROUTER_ADMIN=admin
ROUTER_PASS=password
```

4. Run locally:
```bash
go run main.go
```

## RBAC Permissions

The controller requires the following Kubernetes permissions:

- **Resources**: `services`
- **Verbs**: `get`, `list`, `watch`
- **Scope**: Cluster-wide (all namespaces)

These permissions are defined in the `ClusterRole` in `deployment.yaml`.

## Security Considerations

- The controller runs as a non-root user (UID 1000)
- Minimal RBAC permissions (read-only access to services)
- Router credentials are stored in Kubernetes secrets
- Container runs with dropped capabilities and read-only root filesystem disabled (can be enabled if needed)

## Troubleshooting

### Controller not connecting to router

- Verify `ROUTER_BASE` is accessible from the cluster
- Check router credentials in the secret
- Review controller logs: `kubectl logs -l app=nodeport-router`

### Port forwards not being created

- Ensure services are of type `NodePort`
- Check that NodePort values are non-zero
- Verify the router web interface is accessible
- Review controller logs for errors

### Permission errors

- Verify the ServiceAccount has the correct ClusterRoleBinding
- Check that the ServiceAccount is specified in the deployment

## Limitations

- Service deletion handling is not fully implemented (port forwards are not automatically removed)
- Update logic for existing port forwards may need refinement
- Currently only supports Arris NVG443B router

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

Uh, TBD

## Author

misleb

