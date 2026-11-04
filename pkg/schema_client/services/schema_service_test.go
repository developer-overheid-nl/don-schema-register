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
	jsonschema "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/jsonschema"
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
	require.Nil(t, created.Organisation)
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

func TestGetSchemaFiltersPutsTypeFirstAndOmitsSourceMetaRoot(t *testing.T) {
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
	require.Len(t, groups, 2)
	require.Equal(t, "rootType", groups[0].Key)
	require.Equal(t, "Type", groups[0].Label)
	require.Equal(t, "Het type van het schema.", groups[0].Description)
	require.Equal(t, "dialect", groups[1].Key)
	require.Equal(t, "multi-select", groups[0].Type)
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

const staticBase = "https://static.developer.overheid.nl/schemas/"

func staticFile(path string, raw string) models.StaticSchemaFile {
	return models.StaticSchemaFile{URL: staticBase + path, Raw: []byte(raw)}
}

func TestHarvestStaticSchemasStoresFileURL(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	raw := `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://schemas.example.org/don/Api","type":"object"}`
	file := staticFile("don/Api.schema.json", raw)

	count, err := svc.HarvestStaticSchemas(ctx, models.StaticHarvest{
		BaseURL: staticBase, Files: []models.StaticSchemaFile{file}, ListedURLs: []string{file.URL}, Complete: true,
	})
	require.NoError(t, err)
	require.Equal(t, 1, count)

	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	s := all[0]
	require.NotEmpty(t, s.Id)
	require.Equal(t, file.URL, s.SchemaUrl)
	require.Equal(t, file.URL, s.SourceMetaIdentifier)
	require.Equal(t, "Api", s.SourceMetaName)
	require.Equal(t, "Api", s.Title)
	require.Equal(t, "don/Api", s.SourceMetaPath)
	require.Equal(t, "don", s.SourceMetaRoot)
	require.Equal(t, "2020-12", s.Dialect)
	require.Equal(t, "object", s.RootType)
	require.NotEmpty(t, s.Hash)
	require.Equal(t, "https://schemas.example.org/don/Api", s.Content["$id"])
	require.False(t, s.LastCrawledAt.IsZero())

	// Tweede run vindt hetzelfde record terug.
	_, err = svc.HarvestStaticSchemas(ctx, models.StaticHarvest{
		BaseURL: staticBase, Files: []models.StaticSchemaFile{file}, ListedURLs: []string{file.URL}, Complete: true,
	})
	require.NoError(t, err)
	again, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, again, 1)
	require.Equal(t, s.Id, again[0].Id)
}

func TestHarvestStaticSchemasDeletesVanishedOnlyWhenComplete(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.SaveSchema(ctx, &models.Schema{Id: "x", Title: "X", SourceMetaIdentifier: staticBase + "weg.schema.json"}))
	require.NoError(t, repo.SaveSchema(ctx, &models.Schema{Id: "handmatig", Title: "Handmatig", SchemaUrl: "https://example.org/h.json"}))

	_, err := svc.HarvestStaticSchemas(ctx, models.StaticHarvest{BaseURL: staticBase, Complete: false})
	require.NoError(t, err)
	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	_, err = svc.HarvestStaticSchemas(ctx, models.StaticHarvest{
		BaseURL: staticBase, Complete: true, ListedURLs: []string{staticBase + "ander.schema.json"},
	})
	require.NoError(t, err)
	all, err = repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "handmatig", all[0].Id)
}

func TestHarvestStaticSchemasKeepsRecordWhenFileInvalid(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	url := staticBase + "y.schema.json"
	require.NoError(t, repo.SaveSchema(ctx, &models.Schema{Id: "y", Title: "Oud", Hash: "oud", SourceMetaIdentifier: url}))

	count, err := svc.HarvestStaticSchemas(ctx, models.StaticHarvest{
		BaseURL: staticBase, Files: []models.StaticSchemaFile{staticFile("y.schema.json", "{")},
		ListedURLs: []string{url}, Complete: true,
	})
	require.NoError(t, err)
	require.Equal(t, 0, count)

	got, err := repo.GetSchemaByID(ctx, "y")
	require.NoError(t, err)
	require.Equal(t, "Oud", got.Title)
	require.Equal(t, "oud", got.Hash)
}

func TestHarvestStaticSchemasDoesNotDeleteWhenNothingListed(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.SaveSchema(ctx, &models.Schema{Id: "x", Title: "X", SourceMetaIdentifier: staticBase + "x.schema.json"}))

	_, err := svc.HarvestStaticSchemas(ctx, models.StaticHarvest{BaseURL: staticBase, Complete: true})
	require.NoError(t, err)
	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
}

func TestHarvestStaticSchemasDoesNotTakeOverAPIRecordWithSameContent(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	raw := `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Gelijk","type":"object"}`
	res, err := jsonschema.ParseValidateAndHash([]byte(raw), "application/schema+json")
	require.NoError(t, err)
	require.NoError(t, repo.SaveSchema(ctx, &models.Schema{
		Id: "api", Title: "API", SchemaUrl: "https://example.org/api.json", Hash: res.Hash, Content: res.Content,
	}))

	f := staticFile("a.schema.json", raw)
	_, err = svc.HarvestStaticSchemas(ctx, models.StaticHarvest{
		BaseURL: staticBase, Files: []models.StaticSchemaFile{f}, ListedURLs: []string{f.URL}, Complete: true,
	})
	require.NoError(t, err)

	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	api, err := repo.GetSchemaByID(ctx, "api")
	require.NoError(t, err)
	require.Equal(t, "API", api.Title)
	require.Empty(t, api.SourceMetaIdentifier)
	require.Equal(t, "https://example.org/api.json", api.SchemaUrl)
}

func TestHarvestStaticSchemasKeepsIdenticalFilesSeparate(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	raw := `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`
	a, b := staticFile("a.schema.json", raw), staticFile("b.schema.json", raw)

	count, err := svc.HarvestStaticSchemas(ctx, models.StaticHarvest{
		BaseURL: staticBase, Files: []models.StaticSchemaFile{a, b}, ListedURLs: []string{a.URL, b.URL}, Complete: true,
	})
	require.NoError(t, err)
	require.Equal(t, 2, count)
	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func harvestOne(t *testing.T, svc *services.SchemaService, path, raw string) {
	t.Helper()
	file := staticFile(path, raw)
	_, err := svc.HarvestStaticSchemas(context.Background(), models.StaticHarvest{
		BaseURL: staticBase, Files: []models.StaticSchemaFile{file},
	})
	require.NoError(t, err)
}

func schemaByTitle(t *testing.T, repo schemas.SchemasRepository, title string) models.Schema {
	t.Helper()
	all, err := repo.AllSchemas(context.Background())
	require.NoError(t, err)
	for _, s := range all {
		if s.Title == title {
			return s
		}
	}
	t.Fatalf("schema %q niet gevonden", title)
	return models.Schema{}
}

func TestRefreshAnalysisLinksDependenciesAcrossSources(t *testing.T) {
	svc, repo := newTestService(t)
	ctx := context.Background()
	createTestOrg(t, svc)

	orgURL := "https://schemas.example.org/don/Organisation"
	harvestOne(t, svc, "don/Organisation.schema.json",
		`{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"`+orgURL+`","title":"Organisation","type":"object"}`)

	_, err := svc.CreateSchemaFromInput(models.SchemaPost{
		OrganisationUri: "https://example.org/org",
		Contact:         testContact(),
		SchemaBody: `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://example.org/Client","title":"Client","type":"object",` +
			`"properties":{"org":{"$ref":"` + orgURL + `"}}}`,
	})
	require.NoError(t, err)

	org := schemaByTitle(t, repo, "Organisation")
	client := schemaByTitle(t, repo, "Client")
	require.Equal(t, 1, client.SourceMetaDependencies)
	require.Greater(t, client.SourceMetaHealth, 0)
	require.Len(t, client.SourceMetaDependencyDetails, 1)
	require.Equal(t, org.Id, client.SourceMetaDependencyDetails[0].ToSchemaId)
	require.Equal(t, client.Id, client.SourceMetaDependencyDetails[0].FromSchemaId)

	// Organisation ziet de inkomende edge, maar heeft zelf geen uitgaande.
	require.Equal(t, 0, org.SourceMetaDependencies)
	require.Len(t, org.SourceMetaDependencyDetails, 1)
	require.Equal(t, client.Id, org.SourceMetaDependencyDetails[0].FromSchemaId)
	require.Equal(t, org.Id, org.SourceMetaDependencyDetails[0].ToSchemaId)
	_ = ctx
}

func TestRefreshAnalysisIncomingEdgesFromSeveralSchemas(t *testing.T) {
	svc, repo := newTestService(t)
	base := "https://schemas.example.org/don/"
	draft := `"$schema":"https://json-schema.org/draft/2020-12/schema"`
	harvestOne(t, svc, "don/Organisation.schema.json", `{`+draft+`,"$id":"`+base+`Organisation","title":"Organisation","type":"object"}`)
	harvestOne(t, svc, "don/Api.schema.json", `{`+draft+`,"$id":"`+base+`Api","title":"Api","type":"object","properties":{"organisation":{"$ref":"`+base+`Organisation"}}}`)
	harvestOne(t, svc, "don/Repository.schema.json", `{`+draft+`,"$id":"`+base+`Repository","title":"Repository","type":"object","properties":{"owner":{"$ref":"`+base+`Organisation"}}}`)

	org := schemaByTitle(t, repo, "Organisation")
	require.Equal(t, 0, org.SourceMetaDependencies)
	require.Len(t, org.SourceMetaDependencyDetails, 2)
	api := schemaByTitle(t, repo, "Api")
	repository := schemaByTitle(t, repo, "Repository")
	// Deterministisch: inkomend gesorteerd op From, dan At.
	require.Equal(t, api.Id, org.SourceMetaDependencyDetails[0].FromSchemaId)
	require.Equal(t, "/properties/organisation/$ref", org.SourceMetaDependencyDetails[0].At)
	require.Equal(t, repository.Id, org.SourceMetaDependencyDetails[1].FromSchemaId)
	for _, d := range org.SourceMetaDependencyDetails {
		require.Equal(t, org.Id, d.ToSchemaId)
	}
}

func TestHarvestKeepsRealFileSize(t *testing.T) {
	svc, repo := newTestService(t)
	raw := `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://schemas.example.org/don/A","title":"A","type":"object"}`
	harvestOne(t, svc, "don/A.schema.json", raw)
	require.Equal(t, len(raw), schemaByTitle(t, repo, "A").SourceMetaBytes)
	// Een latere analyse zonder raw laat de bestandsgrootte staan.
	require.NoError(t, svc.RefreshAnalysis(context.Background()))
	require.Equal(t, len(raw), schemaByTitle(t, repo, "A").SourceMetaBytes)
}

func TestRefreshAnalysisReportsUnresolvedForBodyWithoutId(t *testing.T) {
	svc, repo := newTestService(t)
	createTestOrg(t, svc)
	require.NotPanics(t, func() {
		_, err := svc.CreateSchemaFromInput(models.SchemaPost{
			OrganisationUri: "https://example.org/org",
			Contact:         testContact(),
			SchemaBody:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Loos","type":"object","properties":{"x":{"$ref":"Other.schema.json"}}}`,
		})
		require.NoError(t, err)
	})
	s := schemaByTitle(t, repo, "Loos")
	var found bool
	for _, issue := range s.SourceMetaHealthIssues {
		if issue.Name == "unresolved_refs" {
			found = true
		}
	}
	require.True(t, found, "verwacht unresolved_refs, kreeg %+v", s.SourceMetaHealthIssues)
}

func TestCreateSchemaRunsAnalysis(t *testing.T) {
	svc, repo := newTestService(t)
	createTestOrg(t, svc)
	_, err := svc.CreateSchemaFromInput(models.SchemaPost{
		OrganisationUri: "https://example.org/org",
		Contact:         testContact(),
		SchemaBody:      `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"https://example.org/S","title":"S","description":"d","type":"object"}`,
	})
	require.NoError(t, err)
	s := schemaByTitle(t, repo, "S")
	require.Greater(t, s.SourceMetaHealth, 0)
	require.Greater(t, s.SourceMetaBytes, 0)
	require.Equal(t, "https://json-schema.org/draft/2020-12/schema", s.SourceMetaDialect)
}
