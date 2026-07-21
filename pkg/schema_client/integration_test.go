package schema_client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	schema_client "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/handler"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/schemas"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/services"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type integrationEnv struct {
	server  *httptest.Server
	repo    schemas.SchemasRepository
	service *services.SchemaService
	client  *http.Client
}

func newIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Organisation{}, &models.Schema{}))

	repo := schemas.NewSchemasRepository(db)
	svc := services.NewSchemaService(repo)
	controller := handler.NewSchemaController(svc)
	router := schema_client.NewRouter("test-version", controller)

	server := httptest.NewServer(router)
	t.Cleanup(func() { server.Close() })

	return &integrationEnv{
		server:  server,
		repo:    repo,
		service: svc,
		client:  &http.Client{Timeout: 2 * time.Second},
	}
}

func (e *integrationEnv) doRequest(t *testing.T, method, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, e.server.URL+path, nil)
	require.NoError(t, err)
	resp, err := e.client.Do(req)
	require.NoError(t, err)
	return resp
}

func (e *integrationEnv) doJSONRequest(t *testing.T, method, path string, payload any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if payload != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(payload))
	}
	req, err := http.NewRequest(method, e.server.URL+path, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	require.NoError(t, err)
	return resp
}

func decodeBody[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	}()
	var out T
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}

func TestSchemasEndpoints(t *testing.T) {
	env := newIntegrationEnv(t)
	ctx := context.Background()

	org, err := env.service.CreateOrganisation(ctx, &models.Organisation{
		Uri:   "https://example.org/organisations/integration",
		Label: "Integration Org",
	})
	require.NoError(t, err)

	objectSchema := &models.Schema{
		Id:             "schema-object",
		Title:          "Bier",
		Description:    "Het specifieke bier dat een brouwerij produceert",
		Dialect:        "2020-12",
		RootType:       "object",
		Collection:     "demo",
		OrganisationID: &org.Uri,
		SchemaUrl:      "https://example.org/schemas/bier.schema.json",
		Content: map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"title":   "Bier",
			"type":    "object",
		},
		Hash:          "hash-object",
		CreatedAt:     time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
		LastCrawledAt: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, env.repo.SaveSchema(ctx, objectSchema))

	numberSchema := &models.Schema{
		Id:             "schema-number",
		Title:          "Percentage",
		Description:    "Een percentage",
		Dialect:        "oas-3.1",
		RootType:       "number",
		OrganisationID: &org.Uri,
		Content: map[string]any{
			"$schema": "https://spec.openapis.org/oas/3.1/dialect/base",
			"title":   "Percentage",
			"type":    "number",
		},
		Hash:          "hash-number",
		CreatedAt:     time.Date(2024, 5, 11, 12, 0, 0, 0, time.UTC),
		LastCrawledAt: time.Date(2024, 5, 11, 12, 0, 0, 0, time.UTC),
	}
	require.NoError(t, env.repo.SaveSchema(ctx, numberSchema))

	unknownTypeSchema := &models.Schema{
		Id:             "schema-unknown",
		Title:          "Samengesteld",
		Dialect:        "2020-12",
		RootType:       "unknown",
		OrganisationID: &org.Uri,
		Content: map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"allOf":   []any{map[string]any{"type": "object"}},
		},
		Hash: "hash-unknown",
	}
	require.NoError(t, env.repo.SaveSchema(ctx, unknownTypeSchema))

	t.Run("list schemas", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "test-version", resp.Header.Get("API-Version"))
		require.Equal(t, "3", resp.Header.Get("Total-Count"))
		require.Equal(t, "1", resp.Header.Get("Current-Page"))
		require.Equal(t, "20", resp.Header.Get("Per-Page"))
		require.Equal(t, "1", resp.Header.Get("Total-Pages"))

		body := decodeBody[[]models.SchemaSummary](t, resp)
		require.Len(t, body, 3)
		// gesorteerd op titel: Bier, Percentage, Samengesteld
		require.Equal(t, "schema-object", body[0].Id)
		require.Equal(t, "Bier", body[0].Title)
		require.Nil(t, body[0].Organisation)
	})

	t.Run("list schemas pagination headers", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas?perPage=1&page=2")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "3", resp.Header.Get("Total-Count"))
		require.Equal(t, "3", resp.Header.Get("Total-Pages"))
		require.Equal(t, "2", resp.Header.Get("Current-Page"))
		require.Equal(t, "1", resp.Header.Get("Per-Page"))
		require.NotEmpty(t, resp.Header.Get("Link"))

		body := decodeBody[[]models.SchemaSummary](t, resp)
		require.Len(t, body, 1)
		require.Equal(t, "schema-number", body[0].Id)
	})

	t.Run("retrieve schema includes content", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas/schema-object")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "test-version", resp.Header.Get("API-Version"))

		body := decodeBody[models.SchemaDetail](t, resp)
		require.Equal(t, "schema-object", body.Id)
		require.Equal(t, "2020-12", body.Dialect)
		require.Equal(t, "object", body.RootType)
		require.Nil(t, body.Organisation)
		require.NotEmpty(t, body.Content)
		require.Equal(t, "Bier", body.Content["title"])
	})

	t.Run("raw schema content", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas/schema-object/schema.json")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "application/schema+json", resp.Header.Get("Content-Type"))

		body := decodeBody[map[string]any](t, resp)
		require.Equal(t, "Bier", body["title"])
		require.Equal(t, "object", body["type"])
	})

	t.Run("raw schema content not found", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas/does-not-exist/schema.json")
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	})

	t.Run("search schemas via q", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas?q=bier")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "1", resp.Header.Get("Total-Count"))

		body := decodeBody[[]models.SchemaSummary](t, resp)
		require.Len(t, body, 1)
		require.Equal(t, "schema-object", body[0].Id)
	})

	t.Run("filter on dialect", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas?dialect=oas-3.1")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "1", resp.Header.Get("Total-Count"))

		body := decodeBody[[]models.SchemaSummary](t, resp)
		require.Len(t, body, 1)
		require.Equal(t, "schema-number", body[0].Id)
	})

	t.Run("filter on rootType", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas?rootType=object&rootType=number")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "2", resp.Header.Get("Total-Count"))
	})

	t.Run("schema filters groups", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas/filters")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body := decodeBody[[]models.FilterGroup](t, resp)
		require.Len(t, body, 2)
		require.Equal(t, "dialect", body[0].Key)
		require.Equal(t, "rootType", body[1].Key)

		dialectValues := map[string]int{}
		for _, opt := range body[0].Options {
			dialectValues[opt.Value] = opt.Count
		}
		require.Equal(t, 2, dialectValues["2020-12"])
		require.Equal(t, 1, dialectValues["oas-3.1"])
	})

	t.Run("schema filters respect active filters", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/schemas/filters?dialect=oas-3.1")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body := decodeBody[[]models.FilterGroup](t, resp)
		var rootTypeGroup models.FilterGroup
		for _, g := range body {
			if g.Key == "rootType" {
				rootTypeGroup = g
			}
		}
		// alleen het oas-3.1 schema telt mee voor rootType counts
		counts := map[string]int{}
		for _, opt := range rootTypeGroup.Options {
			counts[opt.Value] = opt.Count
		}
		require.Equal(t, 1, counts["number"])
		require.Zero(t, counts["object"])
	})

	t.Run("list organisations", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodGet, "/v1/organisations")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "1", resp.Header.Get("Total-Count"))

		body := decodeBody[[]models.OrganisationSummary](t, resp)
		require.Len(t, body, 1)
		require.Equal(t, org.Uri, body[0].Uri)
	})

	t.Run("create schema from schemaBody", func(t *testing.T) {
		resp := env.doJSONRequest(t, http.MethodPost, "/v1/schemas", map[string]any{
			"schemaBody":      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Adres","type":"object"}`,
			"organisationUri": org.Uri,
			"contact": map[string]string{
				"name":  "Team developer.overheid.nl",
				"email": "developer.overheid@geonovum.nl",
				"url":   "https://developer.overheid.nl",
			},
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		body := decodeBody[models.SchemaSummary](t, resp)
		require.Equal(t, "Adres", body.Title)
		require.Equal(t, "2020-12", body.Dialect)
		require.Equal(t, "object", body.RootType)
		require.NotEmpty(t, body.Id)
	})

	t.Run("create schema requires contact", func(t *testing.T) {
		resp := env.doJSONRequest(t, http.MethodPost, "/v1/schemas", map[string]any{
			"schemaBody":      `{"type":"object"}`,
			"organisationUri": org.Uri,
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)

		type problemResponse struct {
			Status int    `json:"status"`
			Title  string `json:"title"`
			Errors []struct {
				Location string `json:"location"`
			} `json:"errors"`
		}
		body := decodeBody[problemResponse](t, resp)
		require.Equal(t, http.StatusBadRequest, body.Status)
		require.NotEmpty(t, body.Errors)
	})

	t.Run("create schema rejects invalid JSON body", func(t *testing.T) {
		resp := env.doJSONRequest(t, http.MethodPost, "/v1/schemas", map[string]any{
			"schemaBody":      `{not valid json`,
			"organisationUri": org.Uri,
			"contact": map[string]string{
				"name":  "Team",
				"email": "team@example.org",
				"url":   "https://example.org",
			},
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	})

	t.Run("update unknown schema suggests POST", func(t *testing.T) {
		resp := env.doJSONRequest(t, http.MethodPut, "/v1/schemas/does-not-exist", map[string]any{
			"schemaBody":      `{"type":"object"}`,
			"organisationUri": org.Uri,
			"contact": map[string]string{
				"name":  "Team",
				"email": "team@example.org",
				"url":   "https://example.org",
			},
		})
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	})

	t.Run("update schema with wrong organisation is forbidden", func(t *testing.T) {
		resp := env.doJSONRequest(t, http.MethodPut, "/v1/schemas/schema-object", map[string]any{
			"schemaBody":      `{"type":"object"}`,
			"organisationUri": "https://example.org/organisations/other",
			"contact": map[string]string{
				"name":  "Team",
				"email": "team@example.org",
				"url":   "https://example.org",
			},
		})
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	})

	t.Run("update schema with matching organisation", func(t *testing.T) {
		resp := env.doJSONRequest(t, http.MethodPut, "/v1/schemas/schema-object", map[string]any{
			"schemaBody":      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Bier v2","type":"object"}`,
			"organisationUri": org.Uri,
			"contact": map[string]string{
				"name":  "Team",
				"email": "team@example.org",
				"url":   "https://example.org",
			},
		})
		require.Equal(t, http.StatusOK, resp.StatusCode)

		body := decodeBody[models.SchemaSummary](t, resp)
		require.Equal(t, "schema-object", body.Id)
		require.Equal(t, "Bier v2", body.Title)
	})

	t.Run("create organisation validation", func(t *testing.T) {
		resp := env.doJSONRequest(t, http.MethodPost, "/v1/organisations", map[string]string{
			"uri":   "notaurl",
			"label": "Test",
		})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	})

	t.Run("method not allowed returns 405 with RFC7807 envelope", func(t *testing.T) {
		resp := env.doRequest(t, http.MethodPatch, "/v1/schemas")
		require.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		require.Equal(t, "test-version", resp.Header.Get("API-Version"))
		require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

		type problemResponse struct {
			Status int    `json:"status"`
			Title  string `json:"title"`
		}
		body := decodeBody[problemResponse](t, resp)
		require.Equal(t, http.StatusMethodNotAllowed, body.Status)
		require.Equal(t, "Method not allowed", body.Title)
	})

	t.Run("openapi describes schema endpoints", func(t *testing.T) {
		data, err := os.ReadFile("../../api/openapi.json")
		require.NoError(t, err)

		var spec map[string]any
		require.NoError(t, json.Unmarshal(data, &spec))
		paths := spec["paths"].(map[string]any)
		schemasPath := paths["/schemas"].(map[string]any)
		schemasGet := schemasPath["get"].(map[string]any)
		require.Equal(t, "listSchemas", schemasGet["operationId"])
		require.Contains(t, paths, "/schemas/filters")
		require.Contains(t, paths, "/schemas/{id}")
		require.Contains(t, paths, "/schemas/{id}/schema.json")
	})
}
