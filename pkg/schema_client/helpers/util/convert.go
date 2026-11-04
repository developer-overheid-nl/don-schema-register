package util

import (
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

func ToSchemaSummary(schema *models.Schema) models.SchemaSummary {
	return models.SchemaSummary{
		Id:          schema.Id,
		SchemaUrl:   schema.SchemaUrl,
		Title:       schema.Title,
		Description: schema.Description,
		Dialect:     schema.Dialect,
		RootType:    schema.RootType,
		Collection:  schema.Collection,
		Contact: models.Contact{
			Name:  schema.ContactName,
			URL:   schema.ContactUrl,
			Email: schema.ContactEmail,
		},
		CreatedAt:     schema.CreatedAt,
		LastCrawledAt: schema.LastCrawledAt,

		FileName:         schema.FileName,
		Path:             schema.Path,
		BundledSchemaUrl: schema.BundledSchemaUrl,
		Bytes:            schema.Bytes,
		BundledBytes:     schema.BundledBytes,
		DialectUri:       schema.DialectUri,
		Health:           schema.Health,
		DependencyCount:  schema.DependencyCount,
	}
}

func ToSchemaDetail(schema *models.Schema) *models.SchemaDetail {
	detail := &models.SchemaDetail{
		SchemaSummary: ToSchemaSummary(schema),
		Content:       schema.Content,
		Dependencies:  append([]models.Dependency(nil), schema.Dependencies...),
		HealthIssues:  append([]models.HealthIssue(nil), schema.HealthIssues...),
	}
	return detail
}
