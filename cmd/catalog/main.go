// catalog serves the books. It reads from Postgres when DATABASE_URL is set and
// falls back to an in-memory seed when it is not, so the service works in every
// chapter — before and after the one that gives it a database.
package main

import (
	"database/sql"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/engineers-musings/k8s-bookshop/internal/bookshop"
	_ "github.com/lib/pq"
)

var db *sql.DB

func main() {
	h := bookshop.NewHealth()
	mux := http.NewServeMux()
	h.Handle(mux, "catalog")

	if dsn := bookshop.Env("DATABASE_URL", ""); dsn != "" {
		var err error
		db, err = sql.Open("postgres", dsn)
		if err != nil {
			log.Fatalf("catalog: bad DATABASE_URL: %v", err)
		}
		// The database is a separate pod that may not be up yet. Fail readiness
		// rather than the process: a pod that cannot reach its database is not
		// dead, it is not ready.
		go func() {
			for {
				if err := db.Ping(); err != nil {
					log.Printf("catalog: database not reachable: %v", err)
					h.SetReady(false)
				} else {
					h.SetReady(true)
				}
				time.Sleep(3 * time.Second)
			}
		}()
	}

	mux.HandleFunc("/books", func(w http.ResponseWriter, r *http.Request) {
		if bookshop.Poisoned() {
			http.Error(w, "this pod is poisoned", http.StatusInternalServerError)
			return
		}
		books, err := load()
		if err != nil {
			log.Printf("catalog: query failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		bookshop.JSON(w, http.StatusOK, map[string]any{
			"servedBy": bookshop.Env("POD_NAME", "?"),
			"version":  bookshop.Version,
			"source":   source(),
			"books":    books,
		})
	})

	mux.HandleFunc("/books/", func(w http.ResponseWriter, r *http.Request) {
		isbn := strings.TrimPrefix(r.URL.Path, "/books/")
		books, err := load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, b := range books {
			if b.ISBN == isbn {
				bookshop.JSON(w, http.StatusOK, b)
				return
			}
		}
		http.Error(w, "no such book", http.StatusNotFound)
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		bookshop.JSON(w, http.StatusOK, map[string]string{
			"version": bookshop.Version, "pod": bookshop.Env("POD_NAME", "?"),
		})
	})

	bookshop.Serve("catalog", mux)
}

func source() string {
	if db != nil {
		return "postgres"
	}
	return "seed"
}

func load() ([]bookshop.Book, error) {
	if db == nil {
		return bookshop.Seed, nil
	}
	rows, err := db.Query(`SELECT isbn, title, author, price FROM books ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []bookshop.Book
	for rows.Next() {
		var b bookshop.Book
		if err := rows.Scan(&b.ISBN, &b.Title, &b.Author, &b.Price); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
