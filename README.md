# k8s-bookshop

The companion repository for **[Kubernetes from Scratch](https://engineers-musings.dev/blog/series/kubernetes-foundations/)** — a 15-chapter book on the *An Engineer's musings* blog.

Clone it, run one command, and you have the bookshop running on a real three-node Kubernetes cluster.

```bash
make up
```

That creates a kind cluster, builds the images, loads them into the nodes, applies the manifests, and waits for everything to come up. Then:

```
open http://localhost:30080
```

`make down` deletes the cluster.

## What you get

A small shop, deliberately built out of the pieces the book teaches:

| Service | What it is | Why it exists in the book |
|---|---|---|
| `web` | the storefront (HTML) | config from a ConfigMap, a secret rendered masked, the `preStop` hook |
| `catalog` | serves `/books` | reads Postgres when `DATABASE_URL` is set, an in-memory seed when it is not |
| `orders` | `POST /orders` | validates an ISBN by calling `http://catalog` — which is how you prove cluster DNS works |
| `postgres` | `postgres:17-alpine` | a PersistentVolumeClaim, and `strategy: Recreate` |

One image, three binaries. Each Deployment picks its binary with `command:`.

Every service also exposes the switches the book uses to break things on purpose:

| Endpoint | Effect |
|---|---|
| `/healthz` | liveness probe target |
| `/readyz` | readiness probe target |
| `/debug/unready` | start failing readiness — traffic is withdrawn, the pod is **not** restarted |
| `/debug/break` | start failing liveness — the kubelet kills and restarts the container |
| `/debug/eat?mb=200` | allocate and **touch** 200 MB, to earn an honest `OOMKilled` against a 128Mi limit |
| `/debug/slow?ms=3000` | sleep, so a mesh timeout has something to time out |
| `/debug/fail?pct=50` | fail that percentage of requests, interleaved — for retries |
| `/debug/poison` | **this pod** fails everything while **still passing its readiness probe** — the pod Kubernetes will not remove and outlier detection will |

## Trace propagation

`internal/bookshop/bookshop.go` carries a `Propagate` helper and a `traceHeaders` list, used on every
outbound call in `web` and `orders`. It exists because of the one thing a service mesh **cannot** do
for you: the mesh will generate a trace ID and attach it to the request arriving at your pod, but it
cannot reach inside your process and carry that ID from the request you *received* onto the request
you *send*. Only your code knows those two things are related.

Without it, every hop starts a fresh trace and your "distributed trace" is a pile of disconnected
single-span stubs — which is exactly what we measured before adding it.

## Layout

```
cluster/kind.yaml               3 nodes, with :30080 mapped to your machine
Dockerfile                      multi-stage Go build; one image, three binaries
manifests/bookshop/             the app: config, catalog, orders, web, postgres
manifests/overlays/staging/     a Kustomize overlay (chapter 14)
manifests/shelf-controller/     a CRD + RBAC + a controller (Kubernetes chapter 13)
manifests/istio/                the mesh: ServiceAccounts, L4/L7 policy, gateway, resilience
controller/                     alpine + kubectl — the controller's image and its reconcile loop
```

## The mesh

The **[Istio from Scratch](https://engineers-musings.dev/blog/series/istio-foundations/)** series
picks up from this exact cluster. Once the bookshop is running:

```bash
make mesh
```

That installs Gateway API, installs Istio's **ambient** profile, gives each service its own
ServiceAccount, and enrolls the namespace — **without restarting a single application pod**.

```
make down   # delete the whole cluster
```

## Versions

Everything in the book was **run** against these. Kubernetes minors move fast; if you are on a much
later release, expect some output to differ.

| | |
|---|---|
| Kubernetes | 1.36.1 (kind's default node image) |
| kind | 0.32.0 |
| kubectl | 1.36.1 |
| helm | 4.2.3 (chapter 14) |
| Gateway API | 1.4.0, standard channel (chapter 10) |

## Two things that will bite you

**`kind load` is not optional.** kind's nodes do not share your Docker image cache. If you build an
image and skip `make load`, every pod is `ErrImagePull`.

**Do not tag the image `:latest`.** An image tagged `:latest` (or with no tag at all) defaults to
`imagePullPolicy: Always`, so Kubernetes will ignore the image you just loaded onto the node and go
looking for it on Docker Hub, where it does not exist. The manifests pin `bookshop:v1` for exactly
this reason. Chapter 2 reproduces the failure on purpose.
