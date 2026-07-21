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

		SourceMetaName:         schema.SourceMetaName,
		SourceMetaIdentifier:   schema.SourceMetaIdentifier,
		SourceMetaBytes:        schema.SourceMetaBytes,
		SourceMetaBytesBundled: schema.SourceMetaBytesBundled,
		SourceMetaBaseDialect:  schema.SourceMetaBaseDialect,
		SourceMetaDialect:      schema.SourceMetaDialect,
		SourceMetaHealth:       schema.SourceMetaHealth,
		SourceMetaDependencies: schema.SourceMetaDependencies,
	}
}

func ToSchemaDetail(schema *models.Schema) *models.SchemaDetail {
	detail := &models.SchemaDetail{
		SchemaSummary:               ToSchemaSummary(schema),
		Content:                     schema.Content,
		SourceMetaDependencyDetails: append([]models.SourceMetaDependency(nil), schema.SourceMetaDependencyDetails...),
	}
	return detail
}
