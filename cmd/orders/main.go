// orders accepts orders and validates each ISBN against catalog — which makes it
// the service that proves cluster DNS and Services actually work.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/engineers-musings/k8s-bookshop/internal/bookshop"
)

type Order struct {
	ID        int       `json:"id"`
	ISBN      string    `json:"isbn"`
	Title     string    `json:"title"`
	Qty       int       `json:"qty"`
	CreatedAt time.Time `json:"createdAt"`
}

var (
	mu     sync.Mutex
	orders []Order
	client = &http.Client{Timeout: 2 * time.Second}
)

func main() {
	h := bookshop.NewHealth()
	mux := http.NewServeMux()
	h.Handle(mux, "orders")

	catalogURL := bookshop.Env("CATALOG_URL", "http://catalog")

	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			mu.Lock()
			defer mu.Unlock()
			bookshop.JSON(w, http.StatusOK, map[string]any{
				"servedBy": bookshop.Env("POD_NAME", "?"),
				"orders":   orders,
			})
		case http.MethodPost:
			var req struct {
				ISBN string `json:"isbn"`
				Qty  int    `json:"qty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			// Ask catalog whether the book exists. This call goes over the
			// Service, which means over cluster DNS.
			resp, err := client.Get(catalogURL + "/books/" + req.ISBN)
			if err != nil {
				log.Printf("orders: catalog unreachable: %v", err)
				http.Error(w, "catalog unreachable: "+err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				http.Error(w, "unknown isbn", http.StatusBadRequest)
				return
			}
			var book bookshop.Book
			json.NewDecoder(resp.Body).Decode(&book)

			if req.Qty <= 0 {
				req.Qty = 1
			}
			mu.Lock()
			o := Order{ID: len(orders) + 1, ISBN: book.ISBN, Title: book.Title, Qty: req.Qty, CreatedAt: time.Now().UTC()}
			orders = append(orders, o)
			mu.Unlock()
			log.Printf("orders: accepted order for %q x%d", book.Title, req.Qty)
			bookshop.JSON(w, http.StatusCreated, o)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		bookshop.JSON(w, http.StatusOK, map[string]string{
			"version": bookshop.Version, "pod": bookshop.Env("POD_NAME", "?"),
		})
	})

	bookshop.Serve("orders", mux)
}
