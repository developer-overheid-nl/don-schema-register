package services_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	httpclient "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/httpclient"
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

func TestCreateOrganisationUsesTOOILabelWhenAvailable(t *testing.T) {
	svc, repo := newTestService(t)
	tooi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/ld+json")
		_ = json.NewEncoder(w).Encode([]httpclient.TooIGraph{{
			Graph: []httpclient.TooIObject{{
				ID: "https://identifier.overheid.nl/tooi/id/org/1",
				Label: []struct {
					Value    string `json:"@value"`
					Language string `json:"@language"`
				}{{Value: "TOOI label", Language: "nl"}},
			}},
		}})
	}))
	defer tooi.Close()

	prevClient := httpclient.HTTPClient
	httpclient.HTTPClient = &http.Client{Transport: rewriteHostTransport(tooi.URL)}
	t.Cleanup(func() { httpclient.HTTPClient = prevClient })

	created, err := svc.CreateOrganisation(context.Background(), &models.Organisation{
		Uri:   "https://identifier.overheid.nl/tooi/id/org/1",
		Label: "Request label",
	})
	require.NoError(t, err)
	require.Equal(t, "TOOI label", created.Label)

	saved, err := repo.FindOrganisationByURI(context.Background(), created.Uri)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "TOOI label", saved.Label)
}

func TestCreateOrganisationFallsBackToRequestLabelWhenTOOIUnavailable(t *testing.T) {
	svc, repo := newTestService(t)
	var calls int
	tooi := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.NotFound(w, r)
	}))
	defer tooi.Close()

	prevClient := httpclient.HTTPClient
	httpclient.HTTPClient = &http.Client{Transport: rewriteHostTransport(tooi.URL)}
	t.Cleanup(func() { httpclient.HTTPClient = prevClient })

	created, err := svc.CreateOrganisation(context.Background(), &models.Organisation{
		Uri:   "https://identifier.overheid.nl/tooi/id/org/1",
		Label: "Request label",
	})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, "Request label", created.Label)

	saved, err := repo.FindOrganisationByURI(context.Background(), created.Uri)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "Request label", saved.Label)
}

func rewriteHostTransport(targetBase string) http.RoundTripper {
	return &rewriteTransport{
		base:   http.DefaultTransport,
		target: targetBase,
	}
}

type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(t.target)
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	return t.base.RoundTrip(req)
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

func TestHarvestSourceMetaSchemasStoresMetadata(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()

	count, err := svc.HarvestSourceMetaSchemas(ctx, []models.SourceMetaSchemaMetadata{
		{
			Name:         "crs",
			Identifier:   "https://schemas.example.org/api-register/crs",
			Bytes:        2240,
			BytesBundled: 2240,
			BaseDialect:  "https://json-schema.org/draft/2020-12/schema",
			Dialect:      "https://json-schema.org/draft/2020-12/schema",
			Health:       82,
			Dependencies: 0,
			Description:  "Coordinate reference system.",
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, count)

	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)

	schema := all[0]
	require.Equal(t, "crs", schema.Title)
	require.Equal(t, "Coordinate reference system.", schema.Description)
	require.Equal(t, "2020-12", schema.Dialect)
	require.Equal(t, "https://schemas.example.org/api-register/crs", schema.SchemaUrl)
	require.Equal(t, "crs", schema.SourceMetaName)
	require.Equal(t, "https://schemas.example.org/api-register/crs", schema.SourceMetaIdentifier)
	require.Equal(t, 2240, schema.SourceMetaBytes)
	require.Equal(t, 2240, schema.SourceMetaBytesBundled)
	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", schema.SourceMetaBaseDialect)
	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", schema.SourceMetaDialect)
	require.Equal(t, 82, schema.SourceMetaHealth)
	require.Equal(t, 0, schema.SourceMetaDependencies)
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

func TestSearchSchemasRetrieveModelAndListOrganisations(t *testing.T) {
	svc, _ := newTestService(t)
	org := createTestOrg(t, svc)

	created, err := svc.CreateSchemaFromInput(models.SchemaPost{
		SchemaBody:      `{"title":"Bier","type":"object"}`,
		OrganisationUri: org.Uri,
		Contact:         testContact(),
	})
	require.NoError(t, err)

	results, pagination, err := svc.SearchSchemas(context.Background(), &models.ListSchemasSearchParams{
		Page: 1, PerPage: 20, Query: "bier",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 1, pagination.TotalRecords)

	model, err := svc.RetrieveSchemaModel(context.Background(), created.Id)
	require.NoError(t, err)
	require.NotNil(t, model)
	require.Equal(t, created.Id, model.Id)

	orgs, orgPagination, err := svc.ListOrganisations(context.Background(), &models.ListOrganisationsParams{Page: 1, PerPage: 20})
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	require.Equal(t, org.Uri, orgs[0].Uri)
	require.Equal(t, 1, orgPagination.TotalRecords)
}

func TestSearchSchemasRequiresQuery(t *testing.T) {
	svc, _ := newTestService(t)

	_, _, err := svc.SearchSchemas(context.Background(), &models.ListSchemasSearchParams{})
	require.Error(t, err)
	var p problem.ProblemJSON
	require.True(t, errors.As(err, &p))
	require.Equal(t, http.StatusBadRequest, p.Status)
}

func TestPublishAllSchemasToTypesenseDisabled(t *testing.T) {
	t.Setenv("ENABLE_TYPESENSE", "false")
	svc, repo := newTestService(t)
	org := createTestOrg(t, svc)
	require.NoError(t, repo.SaveSchema(context.Background(), &models.Schema{
		Id:             "schema-1",
		Title:          "Bier",
		OrganisationID: &org.Uri,
		Content:        map[string]any{"type": "object"},
		Hash:           "hash-1",
	}))

	require.NoError(t, svc.PublishAllSchemasToTypesense(context.Background()))
}
