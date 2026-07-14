.PHONY: up build load deploy down status mesh mesh-down
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

# --- Istio (the "Istio from Scratch" series picks up from here) ---
# Pinned on purpose. Istio supports only TWO minor releases at a time, so a
# floating install (brew) will drift out of the version this book was run against.
ISTIO_VERSION ?= 1.30.2

mesh:
	# Gateway API is NOT bundled with Istio. Install it FIRST or your Gateways and
	# waypoints will silently never program.
	$(KUBECTL) apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.0/standard-install.yaml
	istioctl install --set profile=ambient --skip-confirmation
	$(KUBECTL) apply -f manifests/istio/serviceaccounts.yaml
	$(KUBECTL) rollout restart -n bookshop deploy/catalog deploy/orders deploy/web
	# One label. No pod restarts. That is the whole pitch of ambient.
	$(KUBECTL) label namespace bookshop istio.io/dataplane-mode=ambient --overwrite
	@echo
	@echo "The bookshop is now in the mesh. Nothing restarted. Check:"
	@echo "  istioctl ztunnel-config workload        # PROTOCOL should be HBONE"
	@echo "  istioctl waypoint apply -n bookshop --enroll-namespace   # opt in to L7"

mesh-down:
	istioctl uninstall --purge -y
	$(KUBECTL) label namespace bookshop istio.io/dataplane-mode-

down:
	kind delete cluster --name k8s-lab
