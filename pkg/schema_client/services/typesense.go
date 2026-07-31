package services

import (
	"context"
	"fmt"
	"strings"

	commontypesense "github.com/developer-overheid-nl/don-register-common/typesense"
	httpclient "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/httpclient"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

const (
	defaultTypesenseCollection    = "schema-register"
	defaultTypesenseDetailBaseURL = "https://schemas.developer.overheid.nl/schemas"
	defaultTypesenseLanguage      = "nl"
	defaultTypesenseItemPriority  = 1
)

func loadSchemaTypesenseConfigFromEnv() commontypesense.Config {
	return commontypesense.LoadConfigFromEnv(commontypesense.Defaults{
		Collection:    defaultTypesenseCollection,
		DetailBaseURL: defaultTypesenseDetailBaseURL,
		Language:      defaultTypesenseLanguage,
		ItemPriority:  defaultTypesenseItemPriority,
		DefaultTags:   []string{"schema-register", "schema"},
	})
}

func schemaTypesenseEnabled() bool {
	return loadSchemaTypesenseConfigFromEnv().Enabled()
}

func publishSchemaToTypesense(ctx context.Context, schema *models.Schema) error {
	if schema == nil {
		return fmt.Errorf("typesense: schema is nil")
	}

	cfg := loadSchemaTypesenseConfigFromEnv()
	if !cfg.Enabled() {
		return commontypesense.ErrDisabled
	}

	return commontypesense.UpsertDocument(ctx, httpclient.HTTPClient, cfg, buildSchemaTypesenseDocument(cfg, schema))
}

func buildSchemaTypesenseDocument(cfg commontypesense.Config, schema *models.Schema) map[string]any {
	doc := commontypesense.BaseDocument(cfg, schema.Id)

	if title := schemaTypesenseTitle(schema); title != "" {
		doc["hierarchy.lvl0"] = title
	}
	if org := schemaTypesenseOrganisationLabel(schema); org != "" {
		doc["hierarchy.lvl1"] = org
	}
	if dialect := strings.TrimSpace(schema.Dialect); dialect != "" {
		doc["hierarchy.lvl2"] = schemaTypesenseLabelWithCode(dialect, models.DialectLabels)
	}
	if rootType := strings.TrimSpace(schema.RootType); rootType != "" {
		doc["hierarchy.lvl3"] = schemaTypesenseLabelWithCode(rootType, models.RootTypeLabels)
	}

	if _, ok := doc["hierarchy.lvl2"]; !ok {
		doc["hierarchy.lvl2"] = "JSON Schema"
	}

	if content := buildSchemaTypesenseContent(schema); content != "" {
		doc["content"] = content
	}

	if tags := buildSchemaTypesenseTags(cfg, schema); len(tags) > 0 {
		doc["tags"] = tags
	}

	return doc
}

func schemaTypesenseTitle(schema *models.Schema) string {
	if title := strings.TrimSpace(schema.Title); title != "" {
		return title
	}
	if schemaURL := strings.TrimSpace(schema.SchemaUrl); schemaURL != "" {
		return schemaURL
	}
	return strings.TrimSpace(schema.Id)
}

func schemaTypesenseOrganisationLabel(schema *models.Schema) string {
	if schema.Organisation != nil {
		if label := strings.TrimSpace(schema.Organisation.Label); label != "" {
			return label
		}
		if uri := strings.TrimSpace(schema.Organisation.Uri); uri != "" {
			return uri
		}
	}
	if schema.OrganisationID != nil {
		return strings.TrimSpace(*schema.OrganisationID)
	}
	return ""
}

func buildSchemaTypesenseContent(schema *models.Schema) string {
	parts := make([]string, 0)

	if title := schemaTypesenseTitle(schema); title != "" {
		parts = append(parts, fmt.Sprintf("Naam: %s", title))
	}
	if desc := strings.TrimSpace(schema.Description); desc != "" {
		parts = append(parts, fmt.Sprintf("Beschrijving: %s", desc))
	}
	if schemaURL := strings.TrimSpace(schema.SchemaUrl); schemaURL != "" {
		parts = append(parts, fmt.Sprintf("Schema URL: %s", schemaURL))
	}
	if org := schemaTypesenseOrganisationLabel(schema); org != "" {
		parts = append(parts, fmt.Sprintf("Organisatie: %s", org))
	}
	if dialect := strings.TrimSpace(schema.Dialect); dialect != "" {
		parts = append(parts, fmt.Sprintf("Dialect: %s", schemaTypesenseLabelWithCode(dialect, models.DialectLabels)))
	}
	if rootType := strings.TrimSpace(schema.RootType); rootType != "" {
		parts = append(parts, fmt.Sprintf("Root type: %s", schemaTypesenseLabelWithCode(rootType, models.RootTypeLabels)))
	}
	if collection := strings.TrimSpace(schema.Collection); collection != "" {
		parts = append(parts, fmt.Sprintf("Collectie: %s", collection))
	}

	if len(parts) == 0 {
		return schemaTypesenseTitle(schema)
	}
	return strings.Join(parts, "\n\n")
}

func buildSchemaTypesenseTags(cfg commontypesense.Config, schema *models.Schema) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(cfg.DefaultTags)+8)

	for _, tag := range cfg.DefaultTags {
		out = commontypesense.AppendUnique(out, tag, seen)
	}

	if schemaID := strings.TrimSpace(schema.Id); schemaID != "" {
		out = commontypesense.AppendUnique(out, fmt.Sprintf("schema-id:%s", schemaID), seen)
	}

	if schema.Organisation != nil {
		out = commontypesense.AppendUnique(out, schema.Organisation.Label, seen)
		out = commontypesense.AppendUnique(out, schema.Organisation.Uri, seen)
	} else if schema.OrganisationID != nil {
		out = commontypesense.AppendUnique(out, *schema.OrganisationID, seen)
	}

	if dialect := strings.TrimSpace(schema.Dialect); dialect != "" {
		out = commontypesense.AppendUnique(out, fmt.Sprintf("dialect:%s", dialect), seen)
	}
	if rootType := strings.TrimSpace(schema.RootType); rootType != "" {
		out = commontypesense.AppendUnique(out, fmt.Sprintf("rootType:%s", rootType), seen)
	}
	if collection := strings.TrimSpace(schema.Collection); collection != "" {
		out = commontypesense.AppendUnique(out, fmt.Sprintf("collection:%s", collection), seen)
	}

	return out
}

func schemaTypesenseLabelWithCode(value string, labels map[string][2]string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if meta, ok := labels[value]; ok {
		return fmt.Sprintf("%s (%s)", meta[0], value)
	}
	return value
}
