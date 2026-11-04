package jobs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func newStaticTestServer(routes map[string]string, failing map[string]bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing[r.URL.Path] {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func TestStaticHarvesterWalksDirectories(t *testing.T) {
	srv := newStaticTestServer(map[string]string{
		"/schemas/":                    `[{"name":"don","type":"directory"},{"name":"adr","type":"directory"}]`,
		"/schemas/don/":                `[{"name":"Api.schema.json","type":"file","size":10},{"name":"README.md","type":"file"}]`,
		"/schemas/don/Api.schema.json": `{"title":"Api"}`,
		"/schemas/adr/":                `[{"name":"2.2","type":"directory"}]`,
		"/schemas/adr/2.2/":            `[{"name":"ProblemDetails.schema.json","type":"file"}]`,
		"/schemas/adr/2.2/ProblemDetails.schema.json": `{"title":"PD"}`,
	}, nil)
	defer srv.Close()

	h := NewStaticHarvester(srv.URL+"/schemas/", srv.Client())
	res, err := h.Harvest(context.Background())
	if err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if !res.Complete {
		t.Fatalf("Complete = false, want true")
	}
	var urls []string
	for _, f := range res.Files {
		urls = append(urls, f.URL)
	}
	sort.Strings(urls)
	want := []string{srv.URL + "/schemas/adr/2.2/ProblemDetails.schema.json", srv.URL + "/schemas/don/Api.schema.json"}
	if len(urls) != 2 || urls[0] != want[0] || urls[1] != want[1] {
		t.Fatalf("files = %v, want %v", urls, want)
	}
	if len(res.ListedURLs) != 2 {
		t.Fatalf("ListedURLs = %v, want 2", res.ListedURLs)
	}
	if res.BaseURL != srv.URL+"/schemas/" {
		t.Fatalf("BaseURL = %q", res.BaseURL)
	}
}

func TestStaticHarvesterMarksIncompleteOnListingError(t *testing.T) {
	srv := newStaticTestServer(map[string]string{
		"/schemas/":                    `[{"name":"don","type":"directory"},{"name":"adr","type":"directory"}]`,
		"/schemas/don/":                `[{"name":"Api.schema.json","type":"file"}]`,
		"/schemas/don/Api.schema.json": `{}`,
	}, map[string]bool{"/schemas/adr/": true})
	defer srv.Close()

	res, err := NewStaticHarvester(srv.URL+"/schemas/", srv.Client()).Harvest(context.Background())
	if err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if res.Complete {
		t.Fatalf("Complete = true, want false")
	}
	if len(res.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(res.Files))
	}
}

func TestStaticHarvesterKeepsUnfetchableFileInListedURLs(t *testing.T) {
	srv := newStaticTestServer(map[string]string{
		"/schemas/": `[{"name":"Kapot.schema.json","type":"file"}]`,
	}, nil)
	defer srv.Close()

	res, err := NewStaticHarvester(srv.URL+"/schemas/", srv.Client()).Harvest(context.Background())
	if err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if len(res.Files) != 0 || len(res.ListedURLs) != 1 {
		t.Fatalf("files=%d listed=%d, want 0 and 1", len(res.Files), len(res.ListedURLs))
	}
}

func TestStaticHarvesterRootListingErrorIsIncomplete(t *testing.T) {
	srv := newStaticTestServer(nil, map[string]bool{"/schemas/": true})
	defer srv.Close()

	res, err := NewStaticHarvester(srv.URL+"/schemas/", srv.Client()).Harvest(context.Background())
	if err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if res.Complete {
		t.Fatalf("Complete = true, want false")
	}
}

func TestStaticHarvesterSkipsRepeatedDirectories(t *testing.T) {
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		if r.URL.Path == "/schemas/" {
			_, _ = w.Write([]byte(`[{"name":"a","type":"directory"},{"name":"a","type":"directory"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	res, err := NewStaticHarvester(srv.URL+"/schemas/", srv.Client()).Harvest(context.Background())
	if err != nil {
		t.Fatalf("Harvest: %v", err)
	}
	if hits["/schemas/a/"] != 1 {
		t.Fatalf("hits /schemas/a/ = %d, want 1", hits["/schemas/a/"])
	}
	if !res.Complete {
		t.Fatalf("Complete = false, want true")
	}
}
