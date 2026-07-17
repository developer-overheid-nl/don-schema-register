package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceMetaListURLFromRoot(t *testing.T) {
	got, err := sourceMetaListURL("https://static.don.projects.digilab.network/schemas/")
	if err != nil {
		t.Fatalf("sourceMetaListURL() error = %v", err)
	}

	want := "https://static.don.projects.digilab.network/schemas/self/v1/api/list"
	if got != want {
		t.Fatalf("sourceMetaListURL() = %q, want %q", got, want)
	}
}

func TestSourceMetaHarvesterCrawlsDirectoriesAndMapsSchemaMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/schemas/self/v1/api/list", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, sourceMetaListResponse{
			Entries: []sourceMetaEntry{
				{
					Name: "api-register",
					Type: "directory",
					Path: "/api-register/",
				},
			},
		})
	})
	mux.HandleFunc("/schemas/self/v1/api/list/api-register/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, sourceMetaListResponse{
			Entries: []sourceMetaEntry{
				{
					Name:         "crs",
					Type:         "schema",
					Path:         "/api-register/crs",
					Identifier:   "https://schemas.example.org/api-register/crs",
					Bytes:        2240,
					BytesBundled: 2240,
					BaseDialect:  "https://json-schema.org/draft/2020-12/schema",
					Dialect:      "https://json-schema.org/draft/2020-12/schema",
					Health:       82,
					Dependencies: 0,
					Description:  "Coordinate reference system.",
				},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	harvester := NewSourceMetaHarvester(server.URL+"/schemas/", server.Client())
	entries, err := harvester.Harvest(context.Background())
	if err != nil {
		t.Fatalf("Harvest() error = %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}

	entry := entries[0]
	if entry.Name != "crs" {
		t.Fatalf("Name = %q, want crs", entry.Name)
	}
	if entry.Identifier != "https://schemas.example.org/api-register/crs" {
		t.Fatalf("Identifier = %q", entry.Identifier)
	}
	if entry.Bytes != 2240 || entry.BytesBundled != 2240 {
		t.Fatalf("bytes = %d bundled = %d", entry.Bytes, entry.BytesBundled)
	}
	if entry.BaseDialect != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("BaseDialect = %q", entry.BaseDialect)
	}
	if entry.Dialect != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("Dialect = %q", entry.Dialect)
	}
	if entry.Health != 82 {
		t.Fatalf("Health = %d", entry.Health)
	}
	if entry.Dependencies != 0 {
		t.Fatalf("Dependencies = %d", entry.Dependencies)
	}
	if entry.Description != "Coordinate reference system." {
		t.Fatalf("Description = %q", entry.Description)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
}
