// Package bookshop holds the small amount of machinery the three bookshop
// services share: config from the environment, health state that a reader can
// deliberately break, and JSON helpers.
package bookshop

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
)

// Version is stamped at build time with -ldflags "-X ...Version=v2".
var Version = "dev"

// Book is the one domain type in the whole shop.
type Book struct {
	ISBN   string  `json:"isbn"`
	Title  string  `json:"title"`
	Author string  `json:"author"`
	Price  float64 `json:"price"`
}

// Seed is the catalog used when no database is configured, so that every
// chapter before the storage chapter still has a working shop.
var Seed = []Book{
	{ISBN: "978-0134494166", Title: "Clean Architecture", Author: "Robert C. Martin", Price: 34.99},
	{ISBN: "978-1492034025", Title: "Designing Data-Intensive Applications", Author: "Martin Kleppmann", Price: 49.99},
	{ISBN: "978-0132350884", Title: "Clean Code", Author: "Robert C. Martin", Price: 39.99},
	{ISBN: "978-1449373320", Title: "Kubernetes: Up and Running", Author: "Kelsey Hightower", Price: 44.99},
}

// Health carries the liveness and readiness state. Both start true and can be
// flipped at runtime through the /debug endpoints, which is how the health
// chapter demonstrates what each probe actually does to a pod.
type Health struct {
	live  atomic.Bool
	ready atomic.Bool
}

func NewHealth() *Health {
	h := &Health{}
	h.live.Store(true)
	h.ready.Store(true)
	return h
}

func (h *Health) SetLive(v bool)  { h.live.Store(v) }
func (h *Health) SetReady(v bool) { h.ready.Store(v) }

// Handle registers /healthz, /readyz and the /debug switches that break them.
func (h *Health) Handle(mux *http.ServeMux, name string) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !h.live.Load() {
			http.Error(w, "unhealthy", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !h.ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ready\n"))
	})
	mux.HandleFunc("/debug/unready", func(w http.ResponseWriter, r *http.Request) {
		h.ready.Store(false)
		log.Printf("%s: readiness flipped to NOT READY", name)
		w.Write([]byte("readiness is now failing\n"))
	})
	mux.HandleFunc("/debug/ready", func(w http.ResponseWriter, r *http.Request) {
		h.ready.Store(true)
		log.Printf("%s: readiness flipped to READY", name)
		w.Write([]byte("readiness is now passing\n"))
	})
	mux.HandleFunc("/debug/break", func(w http.ResponseWriter, r *http.Request) {
		h.live.Store(false)
		log.Printf("%s: liveness flipped to BROKEN — the kubelet should restart this container", name)
		w.Write([]byte("liveness is now failing\n"))
	})
	// Allocate and hold memory, to earn an honest OOMKilled in the scheduling
	// chapter: /debug/eat?mb=200 against a 128Mi limit.
	mux.HandleFunc("/debug/eat", func(w http.ResponseWriter, r *http.Request) {
		mb, _ := strconv.Atoi(r.URL.Query().Get("mb"))
		if mb <= 0 {
			mb = 64
		}
		hog = append(hog, make([]byte, mb<<20))
		for i := range hog[len(hog)-1] {
			hog[len(hog)-1][i] = 1 // touch every page, or the kernel never commits it
		}
		w.Write([]byte("allocated\n"))
	})
}

var hog [][]byte

// Env reads a variable with a fallback, which is the entire configuration story
// until the ConfigMap chapter makes it interesting.
func Env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// JSON writes v as an indented JSON response.
func JSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// Serve starts an HTTP server and logs the identity of the process, so that
// `kubectl logs` shows which pod and which version answered.
func Serve(name string, mux *http.ServeMux) {
	port := Env("PORT", "8080")
	log.Printf("%s %s starting on :%s (pod=%s node=%s)",
		name, Version, port, Env("POD_NAME", "?"), Env("NODE_NAME", "?"))
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
