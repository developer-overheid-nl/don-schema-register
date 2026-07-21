package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

func TestSourceMetaListURLPreservesSchemasBase(t *testing.T) {
	got, err := sourceMetaListURL("https://source-meta.internal/schemas/")
	if err != nil {
		t.Fatalf("sourceMetaListURL() error = %v", err)
	}

	want := "https://source-meta.internal/schemas/self/v1/api/list"
	if got != want {
		t.Fatalf("sourceMetaListURL() = %q, want %q", got, want)
	}
}

func TestSourceMetaSchemaURLPreservesSchemasBase(t *testing.T) {
	got, err := sourceMetaSchemaURL("https://source-meta.internal/schemas/", "/api-register/crs")
	if err != nil {
		t.Fatalf("sourceMetaSchemaURL() error = %v", err)
	}
	if want := "https://source-meta.internal/schemas/api-register/crs"; got != want {
		t.Fatalf("sourceMetaSchemaURL() = %q, want %q", got, want)
	}
}

func TestSourceMetaSchemaURLDoesNotDuplicateSchemasBase(t *testing.T) {
	got, err := sourceMetaSchemaURL("https://source-meta.internal/schemas/", "/schemas/api-register/crs")
	if err != nil {
		t.Fatalf("sourceMetaSchemaURL() error = %v", err)
	}
	if want := "https://source-meta.internal/schemas/api-register/crs"; got != want {
		t.Fatalf("sourceMetaSchemaURL() = %q, want %q", got, want)
	}
}

func TestSourceMetaDirectoryURLDoesNotDuplicateSchemasBase(t *testing.T) {
	got, err := sourceMetaDirectoryURL("https://source-meta.internal/schemas/self/v1/api/list", "/schemas/api-register/")
	if err != nil {
		t.Fatalf("sourceMetaDirectoryURL() error = %v", err)
	}
	if want := "https://source-meta.internal/schemas/self/v1/api/list/api-register/"; got != want {
		t.Fatalf("sourceMetaDirectoryURL() = %q, want %q", got, want)
	}
}

func TestSourceMetaDependenciesURLDoesNotDuplicateSchemasBase(t *testing.T) {
	got, err := sourceMetaDependenciesURL("https://source-meta.internal/schemas/", "/schemas/api-register/crs")
	if err != nil {
		t.Fatalf("sourceMetaDependenciesURL() error = %v", err)
	}
	if want := "https://source-meta.internal/schemas/self/v1/api/schemas/dependencies/api-register/crs"; got != want {
		t.Fatalf("sourceMetaDependenciesURL() = %q, want %q", got, want)
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
					Path: "/schemas/api-register/",
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
					Path:         "/schemas/api-register/crs",
					Identifier:   "https://schemas.example.org/api-register/crs",
					Bytes:        2240,
					BytesBundled: 2240,
					BaseDialect:  "https://json-schema.org/draft/2020-12/schema",
					Dialect:      "https://json-schema.org/draft/2020-12/schema",
					Health:       82,
					Dependencies: 1,
					Description:  "Coordinate reference system.",
				},
			},
		})
	})
	mux.HandleFunc("/schemas/api-register/crs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/schema+json")
		_, _ = w.Write([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"CRS","description":"Schema description.","type":"object"}`))
	})
	mux.HandleFunc("/schemas/self/v1/api/schemas/dependencies/api-register/crs", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, []models.SourceMetaDependency{
			{
				From: "https://schemas.example.org/api-register/crs",
				To:   "https://schemas.example.org/api-register/_shared/link",
				At:   "/properties/links/items/$ref",
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
	if entry.Path != "/schemas/api-register/crs" {
		t.Fatalf("Path = %q", entry.Path)
	}
	if len(entry.RawContent) == 0 {
		t.Fatalf("RawContent is empty")
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
	if entry.Dependencies != 1 {
		t.Fatalf("Dependencies = %d", entry.Dependencies)
	}
	if len(entry.DependencyDetails) != 1 {
		t.Fatalf("DependencyDetails = %#v, want one dependency", entry.DependencyDetails)
	}
	if entry.DependencyDetails[0].To != "https://schemas.example.org/api-register/_shared/link" {
		t.Fatalf("DependencyDetails[0].To = %q", entry.DependencyDetails[0].To)
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
