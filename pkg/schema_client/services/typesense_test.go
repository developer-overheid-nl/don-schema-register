package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	commontypesense "github.com/developer-overheid-nl/don-register-common/typesense"
	httpclient "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/httpclient"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

func TestPublishSchemaToTypesenseDisabled(t *testing.T) {
	t.Setenv("TYPESENSE_ENDPOINT", "")
	t.Setenv("TYPESENSE_API_KEY", "")
	t.Setenv("TYPESENSE_COLLECTION", "")

	err := publishSchemaToTypesense(context.Background(), &models.Schema{Id: "schema-1"})
	if !errors.Is(err, commontypesense.ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestPublishSchemaToTypesenseFeatureFlagDisabled(t *testing.T) {
	t.Setenv("TYPESENSE_ENDPOINT", "https://search.example.org")
	t.Setenv("TYPESENSE_API_KEY", "secret")
	t.Setenv("TYPESENSE_COLLECTION", "schema-register")
	t.Setenv("ENABLE_TYPESENSE", "false")

	err := publishSchemaToTypesense(context.Background(), &models.Schema{Id: "schema-1"})
	if !errors.Is(err, commontypesense.ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestPublishSchemaToTypesenseSendsDocument(t *testing.T) {
	var capturedBody []byte
	var capturedPath, capturedAction, capturedKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAction = r.URL.Query().Get("action")
		capturedKey = r.Header.Get("X-TYPESENSE-API-KEY")
		defer func() {
			if err := r.Body.Close(); err != nil {
				t.Errorf("failed to close request body: %v", err)
			}
		}()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		capturedBody = body
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	t.Setenv("TYPESENSE_ENDPOINT", server.URL)
	t.Setenv("TYPESENSE_API_KEY", "secret")
	t.Setenv("TYPESENSE_COLLECTION", "schema-register")
	t.Setenv("TYPESENSE_DETAIL_BASE_URL", "https://schemas.developer.overheid.nl/schemas")
	t.Setenv("TYPESENSE_LANGUAGE", "nl")
	t.Setenv("TYPESENSE_ITEM_PRIORITY", "5")
	t.Setenv("TYPESENSE_DEFAULT_TAGS", "schema-register,schema")

	prevClient := httpclient.HTTPClient
	httpclient.HTTPClient = server.Client()
	t.Cleanup(func() { httpclient.HTTPClient = prevClient })

	orgURI := "https://example.org/org"
	schema := &models.Schema{
		Id:          "schema-1",
		Title:       "Bier",
		Description: "Een bier",
		Dialect:     "2020-12",
		RootType:    "object",
		Collection:  "demo",
		Organisation: &models.Organisation{
			Uri:   orgURI,
			Label: "Example Org",
		},
		OrganisationID: &orgURI,
	}

	if err := publishSchemaToTypesense(context.Background(), schema); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedPath != "/collections/schema-register/documents" {
		t.Fatalf("unexpected path: %s", capturedPath)
	}
	if capturedAction != "upsert" {
		t.Fatalf("unexpected action: %s", capturedAction)
	}
	if capturedKey != "secret" {
		t.Fatalf("unexpected api key: %s", capturedKey)
	}

	var doc map[string]any
	if err := json.Unmarshal(capturedBody, &doc); err != nil {
		t.Fatalf("failed to unmarshal document: %v", err)
	}

	if doc["id"] != "schema-1" {
		t.Fatalf("unexpected id: %v", doc["id"])
	}
	if doc["hierarchy.lvl0"] != "Bier" {
		t.Fatalf("unexpected lvl0: %v", doc["hierarchy.lvl0"])
	}
	if doc["hierarchy.lvl1"] != "Example Org" {
		t.Fatalf("unexpected lvl1: %v", doc["hierarchy.lvl1"])
	}
	if doc["url"] != "https://schemas.developer.overheid.nl/schemas/schema-1" {
		t.Fatalf("unexpected url: %v", doc["url"])
	}
	content, _ := doc["content"].(string)
	if !strings.Contains(content, "Een bier") {
		t.Fatalf("expected content to contain description, got %q", content)
	}

	tags, _ := doc["tags"].([]any)
	tagSet := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if s, ok := tag.(string); ok {
			tagSet[s] = true
		}
	}
	for _, expected := range []string{"schema-register", "schema", "dialect:2020-12", "rootType:object", "collection:demo"} {
		if !tagSet[expected] {
			t.Fatalf("expected tag %q in %v", expected, tags)
		}
	}
}

func TestPublishSchemaToTypesenseErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	t.Setenv("TYPESENSE_ENDPOINT", server.URL)
	t.Setenv("TYPESENSE_API_KEY", "secret")
	t.Setenv("TYPESENSE_COLLECTION", "schema-register")

	prevClient := httpclient.HTTPClient
	httpclient.HTTPClient = server.Client()
	t.Cleanup(func() { httpclient.HTTPClient = prevClient })

	err := publishSchemaToTypesense(context.Background(), &models.Schema{Id: "schema-1"})
	if err == nil || !strings.Contains(err.Error(), "indexing failed with status 500") {
		t.Fatalf("expected status error, got %v", err)
	}
}
