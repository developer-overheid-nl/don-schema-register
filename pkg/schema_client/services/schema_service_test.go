package services_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	problem "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/problem"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/schemas"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/services"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestService(t *testing.T) (*services.SchemaService, schemas.SchemasRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Organisation{}, &models.Schema{}))
	repo := schemas.NewSchemasRepository(db)
	return services.NewSchemaService(repo), repo
}

func createTestOrg(t *testing.T, svc *services.SchemaService) *models.Organisation {
	t.Helper()
	org, err := svc.CreateOrganisation(context.Background(), &models.Organisation{
		Uri:   "https://example.org/org",
		Label: "Example Org",
	})
	require.NoError(t, err)
	return org
}

func testContact() models.Contact {
	return models.Contact{
		Name:  "Team developer.overheid.nl",
		Email: "developer.overheid@geonovum.nl",
		URL:   "https://developer.overheid.nl",
	}
}

func TestCreateSchemaFromInputWithBody(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	created, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaBody:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Bier","description":"Een bier","type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)
	require.Equal(t, "Bier", created.Title)
	require.Equal(t, "2020-12", created.Dialect)
	require.Equal(t, "object", created.RootType)
	require.NotNil(t, created.Organisation)
	require.Equal(t, org.Uri, created.Organisation.Uri)
	require.NotEmpty(t, created.Id)
}

func TestCreateSchemaFromInputWithURL(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/schema+json")
		_, _ = w.Write([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Remote schema","type":"array"}`))
	}))
	defer server.Close()

	created, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaUrl:       server.URL,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)
	require.Equal(t, "Remote schema", created.Title)
	require.Equal(t, "array", created.RootType)
	require.Equal(t, server.URL, created.SchemaUrl)
}

func TestCreateSchemaFromInputMissingContact(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	_, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaBody:      `{"type":"object"}`,
		OrganisationUri: org.Uri,
	})
	require.Error(t, err)
	var p problem.ProblemJSON
	require.True(t, errors.As(err, &p))
	require.Equal(t, http.StatusBadRequest, p.Status)
	require.Len(t, p.Errors, 3)
}

func TestCreateSchemaFromInputInvalidJSON(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	_, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaBody:      `{not json`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.Error(t, err)
	var p problem.ProblemJSON
	require.True(t, errors.As(err, &p))
	require.Equal(t, http.StatusBadRequest, p.Status)
}

func TestCreateSchemaFromInputUnreachableURL(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaUrl:       server.URL,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.Error(t, err)
	var p problem.ProblemJSON
	require.True(t, errors.As(err, &p))
	require.Equal(t, http.StatusBadRequest, p.Status)
}

func TestUpdateSchemaFromInputUnknownIDNeedsPost(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	_, err := svc.UpdateSchemaFromInput(context.Background(), &models.UpdateSchemaInput{
		Id:              "does-not-exist",
		SchemaBody:      `{"type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, services.ErrNeedsPost))
}

func TestUpdateSchemaFromInputOwnerMismatch(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	created, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaBody:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Bier","type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)

	_, err = svc.UpdateSchemaFromInput(context.Background(), &models.UpdateSchemaInput{
		Id:              created.Id,
		SchemaBody:      `{"type":"object"}`,
		OrganisationUri: "https://example.org/other",
		Contact:         testContact(),
	})
	require.Error(t, err)
	var p problem.ProblemJSON
	require.True(t, errors.As(err, &p))
	require.Equal(t, http.StatusForbidden, p.Status)
}

func TestUpdateSchemaFromInputUpdatesContent(t *testing.T) {
	svc, repo := newTestService(t)
	org := createTestOrg(t, svc)

	created, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaBody:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Bier","type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)

	updated, err := svc.UpdateSchemaFromInput(context.Background(), &models.UpdateSchemaInput{
		Id:              created.Id,
		SchemaBody:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Bier v2","type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)
	require.Equal(t, created.Id, updated.Id)
	require.Equal(t, "Bier v2", updated.Title)

	stored, err := repo.GetSchemaByID(context.Background(), created.Id)
	require.NoError(t, err)
	require.Equal(t, "Bier v2", stored.Title)
	require.Equal(t, "Bier v2", stored.Content["title"])
}

func TestRefreshChangedSchemas(t *testing.T) {
	svc, repo := newTestService(t)
	org := createTestOrg(t, svc)

	content := `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Versie 1","type":"object"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/schema+json")
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	created, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaUrl:       server.URL,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)

	// ongewijzigde inhoud → geen updates
	updated, err := svc.RefreshChangedSchemas(context.Background())
	require.NoError(t, err)
	require.Zero(t, updated)

	// gewijzigde inhoud → één update
	content = `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Versie 2","type":"object"}`
	updated, err = svc.RefreshChangedSchemas(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, updated)

	stored, err := repo.GetSchemaByID(context.Background(), created.Id)
	require.NoError(t, err)
	require.Equal(t, "Versie 2", stored.Title)
}

func TestRefreshChangedSchemasSkipsSchemasWithoutURL(t *testing.T) {
	svc, repo := newTestService(t)
	org := createTestOrg(t, svc)

	require.NoError(t, repo.SaveSchema(context.Background(), &models.Schema{
		Id:             "seeded",
		Title:          "Seeded schema",
		OrganisationID: &org.Uri,
		Content:        map[string]any{"type": "object"},
		Hash:           "seed-hash",
	}))

	updated, err := svc.RefreshChangedSchemas(context.Background())
	require.NoError(t, err)
	require.Zero(t, updated)
}

func TestListSchemasAndRetrieve(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	created, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaBody:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Bier","type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)

	list, pagination, err := svc.ListSchemas(context.Background(), &models.ListSchemasParams{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, 1, pagination.TotalRecords)

	detail, err := svc.RetrieveSchema(context.Background(), created.Id)
	require.NoError(t, err)
	require.NotNil(t, detail)
	require.Equal(t, "Bier", detail.Title)
	require.NotEmpty(t, detail.Content)
}

func TestGetSchemaFiltersGroups(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	_, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaBody:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Bier","type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)

	groups, err := svc.GetSchemaFilters(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, groups, 3)
	require.Equal(t, "dialect", groups[0].Key)
	require.Equal(t, "rootType", groups[1].Key)
	require.Equal(t, "organisation", groups[2].Key)
	require.Equal(t, "multi-select", groups[0].Type)
	require.Equal(t, "single-select", groups[2].Type)
}
