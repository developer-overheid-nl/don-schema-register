package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	problem "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/problem"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/schemas"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestController(t *testing.T) (*SchemaController, schemas.SchemasRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Organisation{}, &models.Schema{}); err != nil {
		t.Fatal(err)
	}
	repo := schemas.NewSchemasRepository(db)
	return NewSchemaController(services.NewSchemaService(repo)), repo
}

func newGinContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(method, target, nil)
	return ctx, rec
}

func seedHandlerSchema(t *testing.T, repo schemas.SchemasRepository) models.Schema {
	t.Helper()
	org := &models.Organisation{Uri: "https://example.org/org", Label: "Example Org"}
	if err := repo.SaveOrganisatie(org); err != nil {
		t.Fatal(err)
	}
	schema := models.Schema{
		Id:             "schema-1",
		Title:          "Bier",
		Description:    "Een bier",
		Dialect:        "2020-12",
		RootType:       "object",
		OrganisationID: &org.Uri,
		Content:        map[string]any{"type": "object"},
		Hash:           "hash-1",
	}
	if err := repo.SaveSchema(context.Background(), &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestListSchemasSetsPaginationHeaders(t *testing.T) {
	controller, repo := newTestController(t)
	seedHandlerSchema(t, repo)
	ctx, rec := newGinContext(http.MethodGet, "/v1/schemas")

	result, err := controller.ListSchemas(ctx, &models.ListSchemasParams{Page: -1, PerPage: 500})
	if err != nil {
		t.Fatalf("ListSchemas() error = %v", err)
	}
	if len(result) != 1 || result[0].Id != "schema-1" {
		t.Fatalf("result = %#v, want seeded schema", result)
	}
	if rec.Header().Get("Total-Count") != "1" || rec.Header().Get("Per-Page") != "100" {
		t.Fatalf("headers = %#v, want pagination headers", rec.Header())
	}
}

func TestRetrieveSchema(t *testing.T) {
	controller, repo := newTestController(t)
	seedHandlerSchema(t, repo)
	ctx, _ := newGinContext(http.MethodGet, "/v1/schemas/schema-1")

	detail, err := controller.RetrieveSchema(ctx, &models.SchemaParams{Id: "schema-1"})
	if err != nil {
		t.Fatalf("RetrieveSchema() error = %v", err)
	}
	if detail == nil || detail.Title != "Bier" {
		t.Fatalf("detail = %#v, want Bier schema", detail)
	}

	_, err = controller.RetrieveSchema(ctx, &models.SchemaParams{Id: "missing"})
	var p problem.ProblemJSON
	if !errors.As(err, &p) || p.Status != http.StatusNotFound {
		t.Fatalf("error = %v, want not found problem", err)
	}
}

func TestRetrieveSchemaContent(t *testing.T) {
	controller, repo := newTestController(t)
	seedHandlerSchema(t, repo)
	ctx, rec := newGinContext(http.MethodGet, "/v1/schemas/schema-1/schema.json")
	ctx.Params = gin.Params{{Key: "id", Value: "schema-1"}}

	controller.RetrieveSchemaContent(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/schema+json" {
		t.Fatalf("content-type = %q, want application/schema+json", contentType)
	}
	if !strings.Contains(rec.Body.String(), `"type": "object"`) {
		t.Fatalf("body = %s, want schema JSON", rec.Body.String())
	}
}

func TestCreateAndListOrganisations(t *testing.T) {
	controller, _ := newTestController(t)
	ctx, rec := newGinContext(http.MethodPost, "/v1/organisations")

	created, err := controller.CreateOrganisation(ctx, &models.Organisation{
		Uri:   "https://example.org/org",
		Label: "Example Org",
	})
	if err != nil {
		t.Fatalf("CreateOrganisation() error = %v", err)
	}
	if created.Uri != "https://example.org/org" {
		t.Fatalf("created = %#v", created)
	}

	orgs, err := controller.ListOrganisations(ctx, &models.ListOrganisationsParams{})
	if err != nil {
		t.Fatalf("ListOrganisations() error = %v", err)
	}
	if len(orgs) != 1 || orgs[0].Uri != created.Uri {
		t.Fatalf("orgs = %#v, want created organisation", orgs)
	}
	if rec.Header().Get("Total-Count") != "1" {
		t.Fatalf("headers = %#v, want total count", rec.Header())
	}
}

func TestListSchemaFilters(t *testing.T) {
	controller, repo := newTestController(t)
	seedHandlerSchema(t, repo)
	ctx, _ := newGinContext(http.MethodGet, "/v1/schemas/filters")

	groups, err := controller.ListSchemaFilters(ctx, &models.SchemaFiltersParams{})
	if err != nil {
		t.Fatalf("ListSchemaFilters() error = %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %#v, want 2 filter groups", groups)
	}
}

func TestCreateSchemaAndUpdateNeedsPost(t *testing.T) {
	controller, repo := newTestController(t)
	org := &models.Organisation{Uri: "https://example.org/org", Label: "Example Org"}
	if err := repo.SaveOrganisatie(org); err != nil {
		t.Fatal(err)
	}
	ctx, _ := newGinContext(http.MethodPost, "/v1/schemas")

	created, err := controller.CreateSchema(ctx, &models.SchemaPost{
		SchemaBody:      `{"title":"Bier","type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         models.Contact{Name: "Team", Email: "team@example.org", URL: "https://example.org/team"},
	})
	if err != nil {
		t.Fatalf("CreateSchema() error = %v", err)
	}
	if created.Title != "Bier" {
		t.Fatalf("created = %#v, want Bier", created)
	}

	_, err = controller.UpdateSchema(ctx, &models.UpdateSchemaInput{
		Id:              "missing",
		SchemaUrl:       "https://example.org/schema.json",
		OrganisationUri: org.Uri,
		Contact:         models.Contact{Name: "Team", Email: "team@example.org", URL: "https://example.org/team"},
	})
	var p problem.ProblemJSON
	if !errors.As(err, &p) || p.Status != http.StatusNotFound {
		t.Fatalf("error = %v, want not found problem", err)
	}
}

func TestNormalizePagination(t *testing.T) {
	page, perPage := normalizePagination(0, 500)
	if page != 1 || perPage != 100 {
		t.Fatalf("normalizePagination() = %d, %d; want 1, 100", page, perPage)
	}
}
