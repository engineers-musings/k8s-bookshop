.PHONY: up build load deploy down status
KUBECTL ?= kubectl

# One command, empty machine to running bookshop.
up: cluster build load deploy status

cluster:
	kind create cluster --config cluster/kind.yaml

build:
	docker build --build-arg VERSION=v1 -t bookshop:v1 .
	docker build --build-arg VERSION=v2 -t bookshop:v2 .

# kind nodes do not share your Docker image cache. Without this, every pod is ErrImagePull.
load:
	kind load docker-image bookshop:v1 bookshop:v2 --name k8s-lab

deploy:
	$(KUBECTL) create namespace bookshop --dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(KUBECTL) create secret generic bookshop-secrets -n bookshop \
		--from-literal=API_KEY=sk-live-abcd1234 \
		--from-literal=PGPASSWORD=s3cret \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(KUBECTL) apply -k manifests/bookshop
	$(KUBECTL) wait -n bookshop --for=condition=available deploy --all --timeout=180s

status:
	@$(KUBECTL) get pods -n bookshop
	@echo
	@echo "The bookshop is at http://localhost:30080"

down:
	kind delete cluster --name k8s-lab
