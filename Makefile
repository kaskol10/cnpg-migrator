.PHONY: dev backend frontend build docker deploy helm-lint helm-template tidy docker-push docker-verify-image docker-test-image

PLATFORM := linux/arm64
IMAGE_NAME := cnpg-migrator

dev:
	@echo "Start backend: make backend"
	@echo "Start frontend: make frontend"

backend:
	cd backend && go run ./cmd/server

frontend:
	cd frontend && npm run dev

build: frontend-build backend-build

frontend-build:
	cd frontend && npm ci 2>/dev/null || npm install && npm run build

backend-build:
	cd backend && go build -o bin/server ./cmd/server

docker:
	docker build --platform $(PLATFORM) --provenance=false -t $(IMAGE_NAME):latest .
	$(MAKE) docker-test-image IMAGE=$(IMAGE_NAME):latest

docker-test-image:
	@test -n "$(IMAGE)" || (echo "IMAGE is required" && exit 1)
	@echo "Image architecture: $$(docker inspect $(IMAGE) --format '{{.Architecture}}')"
	@docker run --rm --platform $(PLATFORM) --entrypoint file $(IMAGE) ./server | grep -q 'ELF.*aarch64' || \
		(echo "ERROR: ./server is not linux/arm64 ELF" && docker run --rm --platform $(PLATFORM) --entrypoint file $(IMAGE) ./server && exit 1)
	@echo "OK: ./server is linux/arm64 ELF inside $(IMAGE)"

docker-push:
	@test -n "$(IMAGE)" || (echo "IMAGE is required, e.g. make docker-push IMAGE=ghcr.io/YOUR_ORG/cnpg-migrator:0.1.0" && exit 1)
	docker build \
		--platform $(PLATFORM) \
		--provenance=false \
		--no-cache \
		-t $(IMAGE) \
		.
	$(MAKE) docker-test-image IMAGE=$(IMAGE)
	docker push $(IMAGE)
	@echo "Pushed $(IMAGE)"

docker-verify-image:
	@test -n "$(IMAGE)" || (echo "IMAGE is required" && exit 1)
	@docker buildx imagetools inspect $(IMAGE) 2>/dev/null || docker inspect $(IMAGE)

helm-lint:
	helm lint k8s/helm/cnpg-migrator

helm-template:
	helm template cnpg-migrator k8s/helm/cnpg-migrator --namespace cnpg-migrator

deploy:
	helm upgrade --install cnpg-migrator k8s/helm/cnpg-migrator \
		--namespace cnpg-migrator \
		--create-namespace \
		$(if $(IMAGE),--set image.repository=$(shell echo $(IMAGE) | cut -d: -f1) --set image.tag=$(shell echo $(IMAGE) | cut -d: -f2),)

tidy:
	cd backend && go mod tidy
