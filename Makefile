# Image URL to use all building/pushing image targets
IMG ?= veneer:latest

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin

## Go installation settings
## Extracts version from toolchain directive in go.mod (e.g., "toolchain go1.24.11" -> "1.24.11")
## Falls back to go directive if toolchain is not present (e.g., "go 1.25.5" -> "1.25.5")
GO_VERSION ?= $(shell grep '^toolchain' go.mod | sed 's/toolchain go//' || true)
ifeq ($(GO_VERSION),)
GO_VERSION := $(shell grep '^go ' go.mod | sed 's/go //')
endif
GO_INSTALL_DIR ?= $(LOCALBIN)/go
GO ?= $(GO_INSTALL_DIR)/bin/go

# Detect OS and architecture for Go download
GOOS_LOCAL ?= $(shell uname -s | tr '[:upper:]' '[:lower:]')
GOARCH_RAW := $(shell uname -m)
ifeq ($(GOARCH_RAW),x86_64)
	GOARCH_LOCAL := amd64
else ifeq ($(GOARCH_RAW),aarch64)
	GOARCH_LOCAL := arm64
else ifeq ($(GOARCH_RAW),arm64)
	GOARCH_LOCAL := arm64
else
	GOARCH_LOCAL := $(GOARCH_RAW)
endif

GO_TARBALL := go$(GO_VERSION).$(GOOS_LOCAL)-$(GOARCH_LOCAL).tar.gz
GO_DOWNLOAD_URL := https://go.dev/dl/$(GO_TARBALL)

## Kind settings
KIND_VERSION ?= 0.27.0
KIND ?= $(LOCALBIN)/kind
KIND_CLUSTER_NAME ?= veneer
KIND_DOWNLOAD_URL := https://kind.sigs.k8s.io/dl/v$(KIND_VERSION)/kind-$(GOOS_LOCAL)-$(GOARCH_LOCAL)

# Package paths for Go commands (avoid ./... which scans into ./bin/go/src)
GO_PACKAGES := ./cmd/... ./pkg/... ./internal/... ./test/...

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: install-go ## Run go fmt against code.
	$(GO) fmt $(GO_PACKAGES)

.PHONY: vet
vet: install-go ## Run go vet against code.
	$(GO) vet $(GO_PACKAGES)

.PHONY: lint
lint: fmt vet ## Run basic linting (fmt + vet). Note: golangci-lint disabled due to Go 1.24 compatibility issues.
	@echo "Linting complete (fmt + vet only). golangci-lint temporarily disabled due to Go version incompatibility."

.PHONY: test
test: deps fmt vet ## Run unit tests.
	$(GO) test $(GO_PACKAGES) -coverprofile cover.out -covermode=atomic

.PHONY: test-e2e
test-e2e: deps ## Run E2E tests (requires Kind cluster).
	KIND_CLUSTER=$(KIND_CLUSTER_NAME) $(GO) test -v -tags=e2e -timeout=20m ./test/e2e/...

.PHONY: cover
cover: install-go ## Display test coverage report
	$(GO) tool cover -func cover.out

.PHONY: coverhtml
coverhtml: install-go ## Generate and open HTML coverage report
	$(GO) tool cover -html cover.out

##@ Build

.PHONY: build
build: deps fmt vet ## Build manager binary.
	$(GO) build -o bin/manager cmd/main.go

.PHONY: run
run: deps fmt vet ## Run a controller from your host (uses config.local.yaml).
	@if [ ! -f config.local.yaml ]; then \
		echo "Error: config.local.yaml not found. Please create it or use VENEER_PROMETHEUS_URL env var."; \
		echo ""; \
		echo "Quick setup:"; \
		echo "  1. Port-forward to Prometheus: kubectl port-forward -n lumina-system svc/lumina-prometheus 9090:9090"; \
		echo "  2. Copy example config: cp config.example.yaml config.local.yaml"; \
		echo "  3. Edit config.local.yaml to set prometheusUrl: http://localhost:9090"; \
		echo "  4. Run: make run"; \
		exit 1; \
	fi
	$(GO) run ./cmd/main.go --config=config.local.yaml

##@ Build Dependencies

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: install-go
install-go: $(LOCALBIN) ## Download and install Go locally into ./bin/go
	@if [ -x "$(GO)" ] && "$(GO)" version | grep -q "go$(GO_VERSION)"; then \
		echo "Go $(GO_VERSION) already installed at $(GO_INSTALL_DIR)"; \
	else \
		echo "Installing Go $(GO_VERSION) to $(GO_INSTALL_DIR)..."; \
		rm -rf "$(GO_INSTALL_DIR)"; \
		curl -fsSL "$(GO_DOWNLOAD_URL)" -o "$(LOCALBIN)/$(GO_TARBALL)"; \
		tar -C "$(LOCALBIN)" -xzf "$(LOCALBIN)/$(GO_TARBALL)"; \
		rm "$(LOCALBIN)/$(GO_TARBALL)"; \
		echo "Go $(GO_VERSION) installed successfully."; \
		echo ""; \
		echo "Add to your shell:"; \
		echo "  export PATH=$(GO_INSTALL_DIR)/bin:\$$PATH"; \
	fi

.PHONY: deps
deps: install-go ## Download Go module dependencies
	$(GO) mod download

.PHONY: install-kind
install-kind: $(LOCALBIN) ## Download and install Kind locally into ./bin/kind
	@if [ -x "$(KIND)" ] && "$(KIND)" version | grep -q "$(KIND_VERSION)"; then \
		echo "Kind $(KIND_VERSION) already installed at $(KIND)"; \
	else \
		echo "Installing Kind $(KIND_VERSION) to $(KIND)..."; \
		curl -fsSL "$(KIND_DOWNLOAD_URL)" -o "$(KIND)"; \
		chmod +x "$(KIND)"; \
		echo "Kind $(KIND_VERSION) installed successfully."; \
	fi

.PHONY: kind-create
kind-create: install-kind ## Create a Kind cluster for development/testing
	@if "$(KIND)" get clusters 2>/dev/null | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' already exists"; \
	else \
		echo "Creating Kind cluster '$(KIND_CLUSTER_NAME)'..."; \
		"$(KIND)" create cluster --name "$(KIND_CLUSTER_NAME)" --wait 5m; \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' created successfully."; \
	fi

.PHONY: kind-delete
kind-delete: install-kind ## Delete the Kind cluster
	@if "$(KIND)" get clusters 2>/dev/null | grep -q "^$(KIND_CLUSTER_NAME)$$"; then \
		echo "Deleting Kind cluster '$(KIND_CLUSTER_NAME)'..."; \
		"$(KIND)" delete cluster --name "$(KIND_CLUSTER_NAME)"; \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' deleted."; \
	else \
		echo "Kind cluster '$(KIND_CLUSTER_NAME)' does not exist"; \
	fi

.PHONY: kind-load
kind-load: install-kind docker-build ## Load the docker image into Kind cluster
	"$(KIND)" load docker-image "$(IMG)" --name "$(KIND_CLUSTER_NAME)"

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

##@ Development Environment

DEV_ENV_DIR := hack/dev-env

.PHONY: dev-env-up
dev-env-up: kind-create ## Deploy full dev environment (LocalStack, Lumina, Prometheus, mock nodes, CRDs)
	@echo "Deploying development environment..."
	@echo ""
	@echo "Step 1/5: Installing Karpenter CRDs..."
	kubectl apply -k $(DEV_ENV_DIR)/crds
	@echo ""
	@echo "Step 2/5: Deploying LocalStack..."
	kubectl apply -k $(DEV_ENV_DIR)/localstack
	@echo ""
	@echo "Step 3/5: Waiting for LocalStack to be ready..."
	kubectl wait --for=condition=available --timeout=120s deployment/localstack -n localstack
	@echo "Waiting for LocalStack to seed data (30s)..."
	@sleep 30
	@echo ""
	@echo "Step 4/5: Deploying Lumina and Prometheus..."
	kubectl apply -k $(DEV_ENV_DIR)/lumina
	kubectl apply -k $(DEV_ENV_DIR)/prometheus
	@echo ""
	@echo "Step 5/5: Creating mock Kubernetes nodes..."
	kubectl apply -k $(DEV_ENV_DIR)/nodes
	@echo ""
	@echo "Waiting for deployments to be ready..."
	kubectl wait --for=condition=available --timeout=120s deployment/lumina-controller -n lumina-system || true
	kubectl wait --for=condition=available --timeout=60s deployment/prometheus -n prometheus
	@echo ""
	@echo "============================================"
	@echo "Development environment is ready!"
	@echo ""
	@echo "To access Prometheus (for Veneer to query):"
	@echo "  kubectl port-forward -n prometheus svc/prometheus 9090:9090"
	@echo ""
	@echo "To access Lumina metrics directly:"
	@echo "  kubectl port-forward -n lumina-system svc/lumina-metrics 8080:8080"
	@echo ""
	@echo "To run Veneer against this environment:"
	@echo "  1. Start port-forward: kubectl port-forward -n prometheus svc/prometheus 9090:9090"
	@echo "  2. Run Veneer: make run"
	@echo "============================================"

.PHONY: dev-env-down
dev-env-down: ## Tear down the dev environment (keeps Kind cluster and CRDs)
	@echo "Tearing down development environment..."
	kubectl delete -k $(DEV_ENV_DIR)/nodes --ignore-not-found
	kubectl delete -k $(DEV_ENV_DIR)/prometheus --ignore-not-found
	kubectl delete -k $(DEV_ENV_DIR)/lumina --ignore-not-found
	kubectl delete -k $(DEV_ENV_DIR)/localstack --ignore-not-found
	@echo "Development environment torn down."
	@echo "Note: Kind cluster '$(KIND_CLUSTER_NAME)' and CRDs still exist. Use 'make kind-delete' to remove everything."

.PHONY: dev-env-restart
dev-env-restart: dev-env-down dev-env-up ## Restart the dev environment

.PHONY: dev-env-status
dev-env-status: ## Show status of dev environment components
	@echo "=== LocalStack ==="
	@kubectl get pods -n localstack 2>/dev/null || echo "Not deployed"
	@echo ""
	@echo "=== Lumina ==="
	@kubectl get pods -n lumina-system 2>/dev/null || echo "Not deployed"
	@echo ""
	@echo "=== Prometheus ==="
	@kubectl get pods -n prometheus 2>/dev/null || echo "Not deployed"
	@echo ""
	@echo "=== Mock Nodes ==="
	@kubectl get nodes -l topology.kubernetes.io/region=us-west-2 2>/dev/null || echo "Not created"

.PHONY: dev-env-logs
dev-env-logs: ## Show logs from Lumina controller
	kubectl logs -n lumina-system -l app=lumina-controller --tail=100 -f
