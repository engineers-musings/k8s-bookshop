.PHONY: up cluster build load deploy status controller mesh mesh-identity mesh-down down
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

# --- The Shelf controller (Kubernetes chapter 13: CRDs and controllers) ---
# The official registry.k8s.io/kubectl image is DISTROLESS and has no shell, so a
# shell-loop controller cannot run in it. This builds a tiny image that has one.
controller:
	docker build -t bookshop-controller:v1 controller/
	kind load docker-image bookshop-controller:v1 --name k8s-lab
	# The controller's whole program is this shell script, mounted as a ConfigMap.
	# (Kustomize will not read a file outside its own directory, so this is generated
	# here rather than by a configMapGenerator — see manifests/shelf-controller/kustomization.yaml.)
	$(KUBECTL) create configmap controller-src -n bookshop \
		--from-file=reconcile.sh=controller/reconcile.sh \
		--dry-run=client -o yaml | $(KUBECTL) apply -f -
	$(KUBECTL) apply -k manifests/shelf-controller
	$(KUBECTL) rollout restart -n bookshop deploy/shelf-controller
	$(KUBECTL) rollout status -n bookshop deploy/shelf-controller --timeout=180s
	@echo
	@echo "Now create a Shelf and watch the controller reconcile it:"
	@echo "  kubectl apply -n bookshop -f manifests/shelf-controller/example-shelf.yaml"
	@echo "  kubectl get shelves -n bookshop        # the BOOKS column is written by the controller"
	@echo "  kubectl get cm shelf-staff-picks -n bookshop -o jsonpath='{.data.count}'"

# --- Istio (the "Istio from Scratch" series picks up from here) ---
# Pinned on purpose. Istio supports only TWO minor releases at a time, so a
# floating install (brew) will drift out of the version this book was run against.
ISTIO_VERSION ?= 1.30.2

mesh:
	# Gateway API is NOT bundled with Istio. Install it FIRST or your Gateways and
	# waypoints will silently never program.
	$(KUBECTL) apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.0/standard-install.yaml
	istioctl install --set profile=ambient --skip-confirmation
	# THE ENROLLMENT. One label. Zero pod restarts -- same pod names, same start
	# times, still one container each. Check for yourself before and after; that
	# is the whole pitch of ambient, and a sidecar mesh cannot do it.
	$(KUBECTL) label namespace bookshop istio.io/dataplane-mode=ambient --overwrite
	@echo
	@echo ">>> Enrolled. Nothing restarted. Verify:  istioctl ztunnel-config workload  (PROTOCOL=HBONE)"
	@echo

mesh-identity:
	# SEPARATE TARGET, and honestly so: this one DOES restart the pods, because
	# changing a pod's serviceAccountName is a change to the pod spec. That is a
	# Kubernetes restart, not a mesh injection -- the mesh never touched the pods.
	# You need it because all three services were running as `default`, so in SPIFFE
	# terms they had the SAME identity and no identity-based policy could tell them
	# apart. Encryption without authorization.
	$(KUBECTL) apply -f manifests/istio/serviceaccounts.yaml
	$(KUBECTL) patch deploy catalog -n bookshop -p '{"spec":{"template":{"spec":{"serviceAccountName":"catalog"}}}}'
	$(KUBECTL) patch deploy orders  -n bookshop -p '{"spec":{"template":{"spec":{"serviceAccountName":"orders"}}}}'
	$(KUBECTL) patch deploy web     -n bookshop -p '{"spec":{"template":{"spec":{"serviceAccountName":"web"}}}}'
	@echo
	@echo ">>> Each service now has its own SPIFFE identity. Verify:"
	@echo "    istioctl ztunnel-config certificate --node k8s-lab-worker"
	@echo "    istioctl waypoint apply -n bookshop --enroll-namespace   # opt in to L7"

mesh-down:
	istioctl uninstall --purge -y
	$(KUBECTL) label namespace bookshop istio.io/dataplane-mode-

down:
	kind delete cluster --name k8s-lab
