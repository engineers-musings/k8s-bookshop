// web is the storefront. It calls catalog and orders, and it renders whatever
// configuration it was given — which makes it the service the ConfigMap and
// Secret chapter picks on.
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/engineers-musings/k8s-bookshop/internal/bookshop"
)

var client = &http.Client{Timeout: 2 * time.Second}

var page = template.Must(template.New("shop").Parse(`<!doctype html>
<title>{{.Greeting}}</title>
<style>
 body{font:16px/1.5 system-ui,sans-serif;max-width:44rem;margin:3rem auto;padding:0 1rem}
 h1{margin-bottom:.2rem} .meta{color:#666;font-size:.85rem}
 li{margin:.4rem 0} .err{color:#b00}
</style>
<h1>{{.Greeting}}</h1>
<p class="meta">web {{.Version}} · pod {{.Pod}} · node {{.Node}} · banner: {{.Banner}}</p>
{{if .Err}}<p class="err">catalog is unreachable: {{.Err}}</p>{{else}}
<ul>{{range .Books}}<li><strong>{{.Title}}</strong> — {{.Author}} · {{$.Currency}}{{printf "%.2f" .Price}}</li>{{end}}</ul>
<p class="meta">catalog {{.CatalogVersion}} served by {{.CatalogPod}} (source: {{.CatalogSource}})</p>
{{end}}
<p class="meta">{{.OrderCount}} order(s) placed · api key: {{.KeyMasked}}</p>
`))

func main() {
	h := bookshop.NewHealth()
	mux := http.NewServeMux()
	h.Handle(mux, "web")

	catalogURL := bookshop.Env("CATALOG_URL", "http://catalog")
	ordersURL := bookshop.Env("ORDERS_URL", "http://orders")

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data := map[string]any{
			"Greeting":  bookshop.Env("GREETING", "The Bookshop"),
			"Currency":  bookshop.Env("CURRENCY", "$"),
			"Version":   bookshop.Version,
			"Pod":       bookshop.Env("POD_NAME", "?"),
			"Node":      bookshop.Env("NODE_NAME", "?"),
			"Banner":    banner(),
			"KeyMasked": mask(bookshop.Env("API_KEY", "")),
		}

		var cat struct {
			ServedBy string          `json:"servedBy"`
			Version  string          `json:"version"`
			Source   string          `json:"source"`
			Books    []bookshop.Book `json:"books"`
		}
		if err := get(catalogURL+"/books", &cat); err != nil {
			data["Err"] = err.Error()
		} else {
			data["Books"] = cat.Books
			data["CatalogPod"] = cat.ServedBy
			data["CatalogVersion"] = cat.Version
			data["CatalogSource"] = cat.Source
		}

		var ord struct {
			Orders []json.RawMessage `json:"orders"`
		}
		if err := get(ordersURL+"/orders", &ord); err == nil {
			data["OrderCount"] = len(ord.Orders)
		} else {
			data["OrderCount"] = "?"
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page.Execute(w, data)
	})

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		bookshop.JSON(w, http.StatusOK, map[string]string{
			"version": bookshop.Version, "pod": bookshop.Env("POD_NAME", "?"),
		})
	})

	bookshop.Serve("web", mux)
}

// banner is read from a file, not the environment — that is the half of the
// ConfigMap story that env vars cannot tell, because a mounted file updates in
// place and an env var never does.
func banner() string {
	b, err := os.ReadFile(bookshop.Env("BANNER_FILE", "/etc/bookshop/banner.txt"))
	if err != nil {
		return "(no banner mounted)"
	}
	return strings.TrimSpace(string(b))
}

func mask(k string) string {
	if k == "" {
		return "(unset)"
	}
	if len(k) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(k)-4) + k[len(k)-4:]
}

func get(url string, into any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}
