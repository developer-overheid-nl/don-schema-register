package schemas

import (
	"context"
	"errors"
	"log"
	"strings"

	commonpagination "github.com/developer-overheid-nl/don-register-common/pagination"
	commonquery "github.com/developer-overheid-nl/don-register-common/query"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"gorm.io/gorm"
)

type SchemasRepository interface {
	GetSchemas(ctx context.Context, page, perPage int, p *models.SchemaFiltersParams) ([]models.Schema, models.Pagination, error)
	GetSchemaByID(ctx context.Context, id string) (*models.Schema, error)
	SaveSchema(ctx context.Context, schema *models.Schema) error
	UpdateSchema(ctx context.Context, schema *models.Schema) error
	SearchSchemas(ctx context.Context, page, perPage int, organisation *string, query string) ([]models.Schema, models.Pagination, error)
	SaveOrganisatie(organisation *models.Organisation) error
	AllSchemas(ctx context.Context) ([]models.Schema, error)
	GetOrganisations(ctx context.Context, page, perPage int) ([]models.Organisation, models.Pagination, error)
	FindOrganisationByURI(ctx context.Context, uri string) (*models.Organisation, error)
	GetSchemaFilterCounts(ctx context.Context, p *models.SchemaFiltersParams) (*models.SchemaFilterCounts, error)
}

type schemasRepository struct {
	db *gorm.DB
}

type schemaFilterMatcher struct {
	params       *models.SchemaFiltersParams
	organisation string
	query        string
}

func NewSchemasRepository(db *gorm.DB) SchemasRepository {
	return &schemasRepository{db: db}
}

func (r *schemasRepository) SaveSchema(ctx context.Context, schema *models.Schema) error {
	var existing models.Schema
	found := false
	if schema.Id != "" {
		if err := r.db.WithContext(ctx).Where("id = ?", schema.Id).First(&existing).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else {
			found = true
		}
	}

	if !found && schema.SourceMetaIdentifier != "" {
		err := r.db.WithContext(ctx).Where("source_meta_identifier = ?", schema.SourceMetaIdentifier).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			log.Printf("SaveSchema: found existing schema for SourceMeta identifier %q with id %s", schema.SourceMetaIdentifier, existing.Id)
			found = true
		}
	}

	if !found && schema.SourceMetaIdentifier == "" && schema.SchemaUrl != "" {
		err := r.db.WithContext(ctx).Where("schema_url = ?", schema.SchemaUrl).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			log.Printf("SaveSchema: found existing schema for url %q with id %s", schema.SchemaUrl, existing.Id)
			found = true
		}
	}

	if !found && schema.Hash != "" {
		err := r.db.WithContext(ctx).Where("schema_hash = ?", schema.Hash).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			log.Printf("SaveSchema: found existing schema for hash %q with id %s", schema.Hash, existing.Id)
			found = true
		}
	}

	if found {
		schema.Id = existing.Id
		if schema.CreatedAt.IsZero() {
			schema.CreatedAt = existing.CreatedAt
		}
		if schema.OrganisationID == nil {
			schema.OrganisationID = existing.OrganisationID
		}

		return r.db.WithContext(ctx).Save(schema).Error
	}

	return r.db.WithContext(ctx).Create(schema).Error
}

func (r *schemasRepository) UpdateSchema(ctx context.Context, schema *models.Schema) error {
	return r.db.WithContext(ctx).Save(schema).Error
}

func (r *schemasRepository) GetSchemas(ctx context.Context, page, perPage int, p *models.SchemaFiltersParams) ([]models.Schema, models.Pagination, error) {
	page, perPage = commonpagination.Normalize(page, perPage, 20, 100)
	matcher := compileSchemaFilters(p)

	var allSchemas []models.Schema
	if err := applySchemaOrdering(
		r.db.WithContext(ctx),
	).Preload("Organisation").Find(&allSchemas).Error; err != nil {
		return nil, models.Pagination{}, err
	}

	filtered := make([]models.Schema, 0, len(allSchemas))
	for _, schema := range allSchemas {
		if schemaMatchesCompiledFilters(schema, matcher, "") {
			filtered = append(filtered, schema)
		}
	}

	totalRecords := len(filtered)
	pagination := commonpagination.New(page, perPage, totalRecords)

	offset := (page - 1) * perPage
	if offset >= totalRecords {
		return []models.Schema{}, pagination, nil
	}

	end := offset + perPage
	if end > totalRecords {
		end = totalRecords
	}

	return filtered[offset:end], pagination, nil
}

func (r *schemasRepository) GetSchemaByID(ctx context.Context, id string) (*models.Schema, error) {
	var schema models.Schema
	if err := r.db.WithContext(ctx).Preload("Organisation").First(&schema, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &schema, nil
}

func (r *schemasRepository) SearchSchemas(ctx context.Context, page, perPage int, organisation *string, query string) ([]models.Schema, models.Pagination, error) {
	trimmed := strings.TrimSpace(query)
	page, perPage = commonpagination.Normalize(page, perPage, 20, 100)
	if trimmed == "" {
		return []models.Schema{}, commonpagination.New(page, perPage, 0), nil
	}

	pattern := "%" + commonquery.EscapeSQLLike(strings.ToLower(trimmed)) + "%"
	applySearchFilters := func(db *gorm.DB) *gorm.DB {
		if organisation != nil && strings.TrimSpace(*organisation) != "" {
			db = db.Where("organisation_id = ?", strings.TrimSpace(*organisation))
		}
		return db.Where("(LOWER(title) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\' OR LOWER(schema_url) LIKE ? ESCAPE '\\')", pattern, pattern, pattern)
	}

	var totalRecords int64
	if err := applySearchFilters(r.db.WithContext(ctx).Model(&models.Schema{})).Count(&totalRecords).Error; err != nil {
		return nil, models.Pagination{}, err
	}

	var matched []models.Schema
	if err := applySchemaOrdering(applySearchFilters(r.db.WithContext(ctx))).
		Preload("Organisation").
		Offset((page - 1) * perPage).
		Limit(perPage).
		Find(&matched).Error; err != nil {
		return nil, models.Pagination{}, err
	}

	return matched, commonpagination.New(page, perPage, int(totalRecords)), nil
}

func (r *schemasRepository) SaveOrganisatie(organisation *models.Organisation) error {
	return r.db.Save(organisation).Error
}

func (r *schemasRepository) AllSchemas(ctx context.Context) ([]models.Schema, error) {
	var allSchemas []models.Schema
	if err := r.db.WithContext(ctx).Preload("Organisation").Find(&allSchemas).Error; err != nil {
		return nil, err
	}
	return allSchemas, nil
}

func (r *schemasRepository) GetOrganisations(ctx context.Context, page, perPage int) ([]models.Organisation, models.Pagination, error) {
	page, perPage = commonpagination.Normalize(page, perPage, 20, 100)
	offset := (page - 1) * perPage

	var organisations []models.Organisation
	if err := r.db.WithContext(ctx).Order("label asc").Offset(offset).Limit(perPage).Find(&organisations).Error; err != nil {
		return nil, models.Pagination{}, err
	}

	var totalRecords int64
	if err := r.db.Model(&models.Organisation{}).Count(&totalRecords).Error; err != nil {
		return nil, models.Pagination{}, err
	}

	return organisations, commonpagination.New(page, perPage, int(totalRecords)), nil
}

func (r *schemasRepository) FindOrganisationByURI(ctx context.Context, uri string) (*models.Organisation, error) {
	var org models.Organisation
	if err := r.db.WithContext(ctx).Where("uri = ?", uri).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

func applySchemaOrdering(db *gorm.DB) *gorm.DB {
	return db.Order("title").Order("id")
}

func (r *schemasRepository) GetSchemaFilterCounts(ctx context.Context, p *models.SchemaFiltersParams) (*models.SchemaFilterCounts, error) {
	matcher := compileSchemaFilters(p)
	var allSchemas []models.Schema
	if err := r.db.WithContext(ctx).
		Preload("Organisation").
		Find(&allSchemas).Error; err != nil {
		return nil, err
	}

	result := &models.SchemaFilterCounts{}

	result.Dialect = commonquery.CountByField(allSchemas,
		func(schema models.Schema) bool { return schemaMatchesCompiledFilters(schema, matcher, "dialect") },
		func(schema models.Schema) string { return schema.Dialect },
	)

	result.RootType = commonquery.CountByField(allSchemas,
		func(schema models.Schema) bool { return schemaMatchesCompiledFilters(schema, matcher, "rootType") },
		func(schema models.Schema) string { return schema.RootType },
	)

	result.Organisation = commonquery.CountByFieldWithLabel(allSchemas,
		func(schema models.Schema) bool {
			return schemaMatchesCompiledFilters(schema, matcher, "organisation") &&
				schema.OrganisationID != nil && *schema.OrganisationID != ""
		},
		func(schema models.Schema) string {
			if schema.OrganisationID != nil {
				return *schema.OrganisationID
			}
			return ""
		},
		func(schema models.Schema) string {
			if schema.Organisation != nil {
				return schema.Organisation.Label
			}
			return ""
		},
	)

	return result, nil
}

func compileSchemaFilters(p *models.SchemaFiltersParams) *schemaFilterMatcher {
	if p == nil {
		p = &models.SchemaFiltersParams{}
	}

	matcher := &schemaFilterMatcher{params: p}
	if p.Organisation != nil {
		matcher.organisation = strings.TrimSpace(*p.Organisation)
	}
	matcher.query = strings.ToLower(strings.TrimSpace(p.Query))

	return matcher
}

func schemaMatchesCompiledFilters(schema models.Schema, matcher *schemaFilterMatcher, exclude string) bool {
	if matcher == nil || matcher.params == nil {
		return true
	}
	p := matcher.params
	if exclude != "organisation" && matcher.organisation != "" {
		if schema.OrganisationID == nil || *schema.OrganisationID != matcher.organisation {
			return false
		}
	}
	if matcher.query != "" && !schemaMatchesQuery(schema, matcher.query) {
		return false
	}
	if exclude != "dialect" && len(p.Dialect) > 0 {
		if !containsStr(p.Dialect, schema.Dialect) {
			return false
		}
	}
	if exclude != "rootType" && len(p.RootType) > 0 {
		if !containsStr(p.RootType, schema.RootType) {
			return false
		}
	}
	return true
}

func schemaMatchesQuery(schema models.Schema, query string) bool {
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(schema.Title), query) ||
		strings.Contains(strings.ToLower(schema.Description), query) ||
		strings.Contains(strings.ToLower(schema.SchemaUrl), query)
}

func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
