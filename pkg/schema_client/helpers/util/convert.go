package util

import (
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

func ToSchemaSummary(schema *models.Schema) models.SchemaSummary {
	var orgSummary *models.OrganisationSummary
	if schema.Organisation != nil {
		orgSummary = &models.OrganisationSummary{
			Uri:   schema.Organisation.Uri,
			Label: schema.Organisation.Label,
		}
	}
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
		Organisation:  orgSummary,
	}
}

func ToSchemaDetail(schema *models.Schema) *models.SchemaDetail {
	detail := &models.SchemaDetail{
		SchemaSummary: ToSchemaSummary(schema),
		Content:       schema.Content,
	}
	return detail
}
