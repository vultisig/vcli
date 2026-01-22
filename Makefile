.PHONY: help init plan apply destroy cluster-setup deploy-all deploy-infra deploy-services test clean
.PHONY: k8s-status k8s-start k8s-stop k8s-restart e2e-test
.PHONY: build start stop status logs

TERRAFORM_DIR := infrastructure/terraform
KUBECONFIG := $(shell pwd)/.kube/config
VCLI := ./local/vcli.sh

help:
	@echo "Vultisig Cluster Management"
	@echo ""
	@echo "Local Development:"
	@echo "  make start               Infra in Docker + services run natively with go run"
	@echo "  make stop                Stop all services and clean all state"
	@echo "  make status              Show container status"
	@echo "  make logs                Show how to view logs"
	@echo ""
	@echo "Infrastructure (Cloud):"
	@echo "  init              Initialize Terraform"
	@echo "  plan              Plan infrastructure changes"
	@echo "  apply             Provision Hetzner VMs"
	@echo "  destroy           Destroy all infrastructure"
	@echo ""
	@echo "Cluster Setup:"
	@echo "  cluster-setup     Install k3s on all nodes"
	@echo ""
	@echo "K8s Deployment (uses production Relay/VultiServer at api.vultisig.com):"
	@echo "  k8s-start         Deploy + verify all services (RECOMMENDED)"
	@echo "  k8s-stop          Graceful shutdown"
	@echo "  k8s-restart       Stop then start"
	@echo "  e2e-test          Run full E2E test"
	@echo "  deploy-secrets    Deploy secrets only"
	@echo ""
	@echo "Testing:"
	@echo "  test-smoke        Run smoke tests"
	@echo ""
	@echo "Utilities:"
	@echo "  logs-verifier     Tail verifier logs"
	@echo "  logs-worker       Tail worker logs"
	@echo "  logs-dca-worker   Tail DCA worker logs"
	@echo "  k8s-status        Show cluster status"
	@echo "  port-forward      Port forward services for local access"
	@echo "  clean             Remove generated files"

# ============== Infrastructure ==============

init:
	cd $(TERRAFORM_DIR) && terraform init

plan:
	cd $(TERRAFORM_DIR) && terraform plan

apply:
	cd $(TERRAFORM_DIR) && terraform apply

destroy:
	cd $(TERRAFORM_DIR) && terraform destroy

# ============== Cluster Setup ==============

cluster-setup:
	./infrastructure/scripts/setup-cluster.sh

# ============== Kubernetes Deployment ==============

deploy-namespaces:
	kubectl apply -f k8s/base/namespaces.yaml

deploy-secrets:
	@if [ -f k8s/secrets.yaml ]; then \
		kubectl apply -f k8s/secrets.yaml; \
	else \
		echo "ERROR: k8s/secrets.yaml not found"; \
		echo "Copy secrets-template.yaml and fill in values:"; \
		echo "  cp k8s/secrets-template.yaml k8s/secrets.yaml"; \
		exit 1; \
	fi

deploy-infra: deploy-namespaces deploy-secrets
	kubectl apply -f k8s/base/infra/postgres/
	kubectl apply -f k8s/base/infra/redis/
	kubectl apply -f k8s/base/infra/minio/
	@echo "Waiting for infrastructure..."
	kubectl -n infra wait --for=condition=ready pod -l app=postgres --timeout=300s
	kubectl -n infra wait --for=condition=ready pod -l app=redis --timeout=120s
	kubectl -n infra wait --for=condition=ready pod -l app=minio --timeout=120s
	@echo "Infrastructure ready"

deploy-verifier:
	kubectl apply -f k8s/base/verifier/
	kubectl -n verifier wait --for=condition=ready pod -l app=verifier --timeout=300s

deploy-dca:
	kubectl apply -f k8s/base/dca/
	kubectl -n plugin-dca wait --for=condition=ready pod -l app=dca --timeout=300s

deploy-monitoring:
	kubectl apply -f k8s/base/monitoring/prometheus/
	kubectl apply -f k8s/base/monitoring/grafana/
	kubectl -n monitoring wait --for=condition=ready pod -l app=prometheus --timeout=120s
	kubectl -n monitoring wait --for=condition=ready pod -l app=grafana --timeout=120s

deploy-services: deploy-verifier deploy-dca deploy-monitoring

deploy-all: deploy-infra deploy-services

# K8s deploy/start/stop scripts
k8s-start: deploy-secrets
	@./infrastructure/scripts/k8s-start.sh

k8s-stop:
	@./infrastructure/scripts/k8s-stop.sh

k8s-restart: k8s-stop k8s-start

e2e-test:
	@./infrastructure/scripts/e2e-test.sh

# ============== Testing ==============

test-smoke:
	./tests/smoke-test.sh

test-partition:
	./tests/network-partition-test.sh help

partition-isolate-relay:
	./tests/network-partition-test.sh isolate-service relay

partition-isolate-worker:
	./tests/network-partition-test.sh isolate-service worker

partition-restore:
	./tests/network-partition-test.sh restore

# ============== Utilities ==============

logs-verifier:
	kubectl -n verifier logs -l app=verifier,component=api -f

logs-worker:
	kubectl -n verifier logs -l app=verifier,component=worker -f

logs-dca-worker:
	kubectl -n plugin-dca logs -l app=dca,component=worker -f

port-forward:
	@echo "Starting port forwards..."
	@echo "  Verifier:   http://localhost:8080"
	@echo "  Grafana:    http://localhost:3000"
	@echo "  Prometheus: http://localhost:9090"
	@echo "  MinIO:      http://localhost:9000"
	@echo ""
	kubectl -n verifier port-forward svc/verifier 8080:8080 &
	kubectl -n monitoring port-forward svc/grafana 3000:3000 &
	kubectl -n monitoring port-forward svc/prometheus 9090:9090 &
	kubectl -n infra port-forward svc/minio 9000:9000 &
	@echo "Press Ctrl+C to stop all port forwards"
	@wait

k8s-status:
	@echo "=== Cluster Status ==="
	@echo ""
	@echo "Nodes:"
	@kubectl get nodes -o wide
	@echo ""
	@echo "Pods:"
	@kubectl get pods --all-namespaces
	@echo ""
	@echo "Services:"
	@kubectl get svc --all-namespaces

clean:
	rm -rf .kube/
	rm -f setup-env.sh
	rm -rf infrastructure/terraform/.terraform
	rm -f infrastructure/terraform/terraform.tfstate*

# ============== Local Development ==============

COMPOSE_FILE := local/docker-compose.yaml

build-vcli:
	@echo "Building vcli..."
	cd local && go build -o vcli ./cmd/vcli
	@echo "Built: local/vcli"
	@echo "Use ./local/vcli.sh (wrapper) or make start/stop/status"

start:
	@echo "============================================"
	@echo "  Vultisig Local Dev Environment"
	@echo "============================================"
	@echo ""
	@if [ ! -d "../verifier" ]; then \
		echo "ERROR: ../verifier directory not found"; \
		echo "Required sibling repos: vcli, verifier, feeplugin, app-recurring"; \
		exit 1; \
	fi
	@if [ ! -d "../feeplugin" ]; then \
		echo "ERROR: ../feeplugin directory not found"; \
		echo "Required sibling repos: vcli, verifier, feeplugin, app-recurring"; \
		exit 1; \
	fi
	@if [ ! -d "../app-recurring" ]; then \
		echo "ERROR: ../app-recurring directory not found"; \
		echo "Required sibling repos: vcli, verifier, feeplugin, app-recurring"; \
		exit 1; \
	fi
	@echo "Starting infrastructure (postgres, redis, minio)..."
	@docker compose -f $(COMPOSE_FILE) down -v --remove-orphans 2>/dev/null || true
	docker compose -f $(COMPOSE_FILE) up -d
	@echo ""
	@echo "Waiting for infrastructure..."
	@sleep 5
	@echo ""
	@echo "Starting services locally..."
	@./local/scripts/run-services.sh

stop:
	@echo "Stopping all services..."
	@-pkill -9 -f "go run.*cmd/verifier" 2>/dev/null || true
	@-pkill -9 -f "go run.*cmd/worker" 2>/dev/null || true
	@-pkill -9 -f "go run.*cmd/server" 2>/dev/null || true
	@-pkill -9 -f "go run.*cmd/scheduler" 2>/dev/null || true
	@-pkill -9 -f "go run.*cmd/tx_indexer" 2>/dev/null || true
	@-pkill -9 -f "go-build.*/verifier$$" 2>/dev/null || true
	@-pkill -9 -f "go-build.*/worker$$" 2>/dev/null || true
	@-pkill -9 -f "go-build.*/server$$" 2>/dev/null || true
	@-pkill -9 -f "go-build.*/scheduler$$" 2>/dev/null || true
	@-pkill -9 -f "go-build.*/tx_indexer$$" 2>/dev/null || true
	@sleep 2
	@docker exec vultisig-redis redis-cli -a vultisig FLUSHALL 2>/dev/null || true
	@docker compose -f local/docker-compose.yaml down -v --remove-orphans 2>/dev/null || true
	@docker volume rm local_postgres-data local_redis-data local_minio-data 2>/dev/null || true
	@rm -rf ~/.vultisig/vaults/ 2>/dev/null || true
	@echo "Stopped and cleaned."

status:
	@docker compose -f $(COMPOSE_FILE) ps

logs:
	@echo "Services run natively - logs are in local/logs/"
	@echo ""
	@echo "  tail -f local/logs/verifier.log"
	@echo "  tail -f local/logs/worker.log"
	@echo "  tail -f local/logs/dca-server.log"
	@echo "  tail -f local/logs/dca-worker.log"
	@echo "  tail -f local/logs/dca-scheduler.log"
	@echo "  tail -f local/logs/fee-server.log"
	@echo "  tail -f local/logs/fee-worker.log"
	@echo ""
	@echo "All logs: tail -f local/logs/*.log"

