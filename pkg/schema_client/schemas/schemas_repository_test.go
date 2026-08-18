package schemas

import (
	"context"
	"testing"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestRepo(t *testing.T) SchemasRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Organisation{}, &models.Schema{}))
	return NewSchemasRepository(db)
}

func seedTestSchemas(t *testing.T, repo SchemasRepository) *models.Organisation {
	t.Helper()
	ctx := context.Background()

	org := &models.Organisation{Uri: "https://example.org/org", Label: "Example Org"}
	require.NoError(t, repo.SaveOrganisatie(org))

	for _, schema := range []*models.Schema{
		{
			Id: "a", Title: "Adres", Dialect: "2020-12", RootType: "object",
			OrganisationID: &org.Uri, Hash: "hash-a",
			SourceMetaPath: "api-register/demo/adres", SourceMetaRoot: "api-register",
			Content: map[string]any{"type": "object"},
		},
		{
			Id: "b", Title: "Bier", Description: "Een bier", Dialect: "2020-12", RootType: "object",
			OrganisationID: &org.Uri, Hash: "hash-b",
			SourceMetaPath: "api-register/demo/bier", SourceMetaRoot: "api-register",
			Content: map[string]any{"type": "object"},
		},
		{
			Id: "c", Title: "Percentage", Dialect: "oas-3.1", RootType: "number",
			OrganisationID: &org.Uri, Hash: "hash-c",
			SourceMetaPath: "demo/percentage", SourceMetaRoot: "demo",
			Content: map[string]any{"type": "number"},
		},
	} {
		require.NoError(t, repo.SaveSchema(ctx, schema))
	}

	return org
}

func TestGetSchemasPagination(t *testing.T) {
	repo := newTestRepo(t)
	seedTestSchemas(t, repo)
	ctx := context.Background()

	result, pagination, err := repo.GetSchemas(ctx, 1, 2, nil)
	require.NoError(t, err)
	require.Len(t, result, 2)
	require.Equal(t, 3, pagination.TotalRecords)
	require.Equal(t, 2, pagination.TotalPages)
	require.Equal(t, 1, pagination.CurrentPage)
	require.NotNil(t, pagination.Next)
	require.Nil(t, pagination.Previous)
	// gesorteerd op titel
	require.Equal(t, "Adres", result[0].Title)
	require.Equal(t, "Bier", result[1].Title)

	result, pagination, err = repo.GetSchemas(ctx, 2, 2, nil)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Nil(t, pagination.Next)
	require.NotNil(t, pagination.Previous)
	require.Equal(t, "Percentage", result[0].Title)
}

func TestGetSchemasFilters(t *testing.T) {
	repo := newTestRepo(t)
	org := seedTestSchemas(t, repo)
	ctx := context.Background()

	t.Run("dialect", func(t *testing.T) {
		result, pagination, err := repo.GetSchemas(ctx, 1, 20, &models.SchemaFiltersParams{Dialect: []string{"oas-3.1"}})
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, "c", result[0].Id)
		require.Equal(t, 1, pagination.TotalRecords)
	})

	t.Run("rootType", func(t *testing.T) {
		result, _, err := repo.GetSchemas(ctx, 1, 20, &models.SchemaFiltersParams{RootType: []string{"object"}})
		require.NoError(t, err)
		require.Len(t, result, 2)
	})

	t.Run("query", func(t *testing.T) {
		result, _, err := repo.GetSchemas(ctx, 1, 20, &models.SchemaFiltersParams{Query: "bier"})
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, "b", result[0].Id)
	})

	t.Run("organisation", func(t *testing.T) {
		result, _, err := repo.GetSchemas(ctx, 1, 20, &models.SchemaFiltersParams{Organisation: &org.Uri})
		require.NoError(t, err)
		require.Len(t, result, 3)

		other := "https://example.org/other"
		result, _, err = repo.GetSchemas(ctx, 1, 20, &models.SchemaFiltersParams{Organisation: &other})
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("combined", func(t *testing.T) {
		result, _, err := repo.GetSchemas(ctx, 1, 20, &models.SchemaFiltersParams{
			Dialect:  []string{"2020-12"},
			RootType: []string{"object"},
			Query:    "adres",
		})
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, "a", result[0].Id)
	})
}

func TestGetSchemaFilterCounts(t *testing.T) {
	repo := newTestRepo(t)
	org := seedTestSchemas(t, repo)
	ctx := context.Background()

	counts, err := repo.GetSchemaFilterCounts(ctx, nil)
	require.NoError(t, err)

	dialects := map[string]int{}
	for _, fc := range counts.Dialect {
		dialects[fc.Value] = fc.Count
	}
	require.Equal(t, 2, dialects["2020-12"])
	require.Equal(t, 1, dialects["oas-3.1"])

	rootTypes := map[string]int{}
	for _, fc := range counts.RootType {
		rootTypes[fc.Value] = fc.Count
	}
	require.Equal(t, 2, rootTypes["object"])
	require.Equal(t, 1, rootTypes["number"])

	require.Len(t, counts.Organisation, 1)
	require.Equal(t, org.Uri, counts.Organisation[0].Value)
	require.Equal(t, 3, counts.Organisation[0].Count)
}

func TestGetSchemaFilterCountsExcludesOwnGroup(t *testing.T) {
	repo := newTestRepo(t)
	seedTestSchemas(t, repo)
	ctx := context.Background()

	// met een actief dialect-filter blijven de dialect-counts volledig,
	// maar tellen rootType-counts alleen matchende schemas
	counts, err := repo.GetSchemaFilterCounts(ctx, &models.SchemaFiltersParams{Dialect: []string{"oas-3.1"}})
	require.NoError(t, err)

	dialects := map[string]int{}
	for _, fc := range counts.Dialect {
		dialects[fc.Value] = fc.Count
	}
	require.Equal(t, 2, dialects["2020-12"])
	require.Equal(t, 1, dialects["oas-3.1"])

	rootTypes := map[string]int{}
	for _, fc := range counts.RootType {
		rootTypes[fc.Value] = fc.Count
	}
	require.Zero(t, rootTypes["object"])
	require.Equal(t, 1, rootTypes["number"])

}

func TestSaveSchemaIdempotentOnHash(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	first := &models.Schema{Id: "x", Title: "Eerste", Hash: "same-hash", Content: map[string]any{"type": "object"}}
	require.NoError(t, repo.SaveSchema(ctx, first))

	// zelfde hash, ander id → bestaand record wordt bijgewerkt
	second := &models.Schema{Id: "y", Title: "Tweede", Hash: "same-hash", Content: map[string]any{"type": "object"}}
	require.NoError(t, repo.SaveSchema(ctx, second))
	require.Equal(t, "x", second.Id)

	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "Tweede", all[0].Title)
}

func TestSaveSchemaIdempotentOnURL(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	first := &models.Schema{Id: "x", Title: "Eerste", SchemaUrl: "https://example.org/s.json", Hash: "hash-1", Content: map[string]any{}}
	require.NoError(t, repo.SaveSchema(ctx, first))

	second := &models.Schema{Id: "", Title: "Tweede", SchemaUrl: "https://example.org/s.json", Hash: "hash-2", Content: map[string]any{}}
	require.NoError(t, repo.SaveSchema(ctx, second))
	require.Equal(t, "x", second.Id)

	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
}

func TestSaveSchemaIdempotentOnSourceMetaIdentifier(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	first := &models.Schema{
		Id:                   "x",
		Title:                "Eerste",
		SourceMetaIdentifier: "https://schemas.example.org/schema",
		SourceMetaHealth:     80,
	}
	require.NoError(t, repo.SaveSchema(ctx, first))

	second := &models.Schema{
		Id:                   "y",
		Title:                "Tweede",
		SourceMetaIdentifier: "https://schemas.example.org/schema",
		SourceMetaHealth:     90,
	}
	require.NoError(t, repo.SaveSchema(ctx, second))
	require.Equal(t, "x", second.Id)

	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "Tweede", all[0].Title)
	require.Equal(t, 90, all[0].SourceMetaHealth)
}

func TestSaveSchemaMigratesLegacySourceMetaID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	first := &models.Schema{
		Id:                   "source-meta-06aaddfcede69a1f",
		Title:                "Legacy",
		SourceMetaIdentifier: "https://schemas.example.org/schema",
	}
	require.NoError(t, repo.SaveSchema(ctx, first))

	second := &models.Schema{
		Id:                   "opaque-id",
		Title:                "Nieuwe ID",
		SourceMetaIdentifier: "https://schemas.example.org/schema",
	}
	require.NoError(t, repo.SaveSchema(ctx, second))
	require.Equal(t, "opaque-id", second.Id)

	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "opaque-id", all[0].Id)
	require.Equal(t, "Nieuwe ID", all[0].Title)
}

func TestSaveSourceMetaSchemaDoesNotOverwriteManualSchemaByURL(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	manual := &models.Schema{
		Id:        "manual",
		Title:     "Handmatig",
		SchemaUrl: "https://schemas.example.org/schema",
		Hash:      "hash-1",
		Content:   map[string]any{"type": "object"},
	}
	require.NoError(t, repo.SaveSchema(ctx, manual))

	sourceMeta := &models.Schema{
		Id:                   "source-meta",
		Title:                "SourceMeta",
		SchemaUrl:            "https://schemas.example.org/schema",
		SourceMetaIdentifier: "https://schemas.example.org/schema",
		SourceMetaHealth:     90,
	}
	require.NoError(t, repo.SaveSchema(ctx, sourceMeta))

	all, err := repo.AllSchemas(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
}

func TestGetSchemaByIDNotFound(t *testing.T) {
	repo := newTestRepo(t)
	schema, err := repo.GetSchemaByID(context.Background(), "nope")
	require.NoError(t, err)
	require.Nil(t, schema)
}

func TestSearchSchemas(t *testing.T) {
	repo := newTestRepo(t)
	seedTestSchemas(t, repo)
	ctx := context.Background()

	result, pagination, err := repo.SearchSchemas(ctx, 1, 20, nil, "bier")
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "b", result[0].Id)
	require.Equal(t, 1, pagination.TotalRecords)

	result, _, err = repo.SearchSchemas(ctx, 1, 20, nil, "")
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestOrganisationsAndUpdateSchema(t *testing.T) {
	repo := newTestRepo(t)
	org := seedTestSchemas(t, repo)
	ctx := context.Background()

	orgs, pagination, err := repo.GetOrganisations(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	require.Equal(t, org.Uri, orgs[0].Uri)
	require.Equal(t, 1, pagination.TotalRecords)

	found, err := repo.FindOrganisationByURI(ctx, org.Uri)
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, org.Label, found.Label)

	missing, err := repo.FindOrganisationByURI(ctx, "https://example.org/missing")
	require.NoError(t, err)
	require.Nil(t, missing)

	schema, err := repo.GetSchemaByID(ctx, "a")
	require.NoError(t, err)
	schema.Title = "Adres v2"
	require.NoError(t, repo.UpdateSchema(ctx, schema))

	updated, err := repo.GetSchemaByID(ctx, "a")
	require.NoError(t, err)
	require.Equal(t, "Adres v2", updated.Title)
}
