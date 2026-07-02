package typesense

import (
	"context"
	"fmt"
	"strings"

	httpclient "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/httpclient"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	commontypesense "github.com/developer-overheid-nl/don-register-common/typesense"
)

const (
	defaultCollection    = "schema-register"
	defaultDetailBaseURL = "https://schemas.developer.overheid.nl/schemas"
	defaultLanguage      = "nl"
	defaultItemPriority  = 1
)

// ErrDisabled is returned when Typesense configuration is missing.
var ErrDisabled = commontypesense.ErrDisabled

type config = commontypesense.Config

func loadConfigFromEnv() config {
	return commontypesense.LoadConfigFromEnv(commontypesense.Defaults{
		Collection:    defaultCollection,
		DetailBaseURL: defaultDetailBaseURL,
		Language:      defaultLanguage,
		ItemPriority:  defaultItemPriority,
		DefaultTags:   []string{"schema-register", "schema"},
	})
}

// Enabled reports whether Typesense indexing is active based on env vars.
func Enabled() bool {
	return loadConfigFromEnv().Enabled()
}

// PublishSchema pushes the provided schema to Typesense for full-text search.
func PublishSchema(ctx context.Context, schema *models.Schema) (err error) {
	if schema == nil {
		return fmt.Errorf("typesense: schema is nil")
	}

	cfg := loadConfigFromEnv()
	if !cfg.Enabled() {
		return ErrDisabled
	}

	return commontypesense.UpsertDocument(ctx, httpclient.HTTPClient, cfg, buildDocument(cfg, schema))
}

func buildDocument(cfg config, schema *models.Schema) map[string]any {
	doc := commontypesense.BaseDocument(cfg, schema.Id)

	if title := schemaTitle(schema); title != "" {
		doc["hierarchy.lvl0"] = title
	}
	if org := schemaOrganisationLabel(schema); org != "" {
		doc["hierarchy.lvl1"] = org
	}
	if dialect := strings.TrimSpace(schema.Dialect); dialect != "" {
		doc["hierarchy.lvl2"] = labelWithCode(dialect, models.DialectLabels)
	}
	if rootType := strings.TrimSpace(schema.RootType); rootType != "" {
		doc["hierarchy.lvl3"] = labelWithCode(rootType, models.RootTypeLabels)
	}

	if _, ok := doc["hierarchy.lvl2"]; !ok {
		doc["hierarchy.lvl2"] = "JSON Schema"
	}

	if content := buildContent(schema); content != "" {
		doc["content"] = content
	}

	if tags := buildTags(cfg, schema); len(tags) > 0 {
		doc["tags"] = tags
	}

	return doc
}

func schemaTitle(schema *models.Schema) string {
	if title := strings.TrimSpace(schema.Title); title != "" {
		return title
	}
	if schemaURL := strings.TrimSpace(schema.SchemaUrl); schemaURL != "" {
		return schemaURL
	}
	return strings.TrimSpace(schema.Id)
}

func schemaOrganisationLabel(schema *models.Schema) string {
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

func buildContent(schema *models.Schema) string {
	parts := make([]string, 0)

	if title := schemaTitle(schema); title != "" {
		parts = append(parts, fmt.Sprintf("Naam: %s", title))
	}
	if desc := strings.TrimSpace(schema.Description); desc != "" {
		parts = append(parts, fmt.Sprintf("Beschrijving: %s", desc))
	}
	if schemaURL := strings.TrimSpace(schema.SchemaUrl); schemaURL != "" {
		parts = append(parts, fmt.Sprintf("Schema URL: %s", schemaURL))
	}
	if org := schemaOrganisationLabel(schema); org != "" {
		parts = append(parts, fmt.Sprintf("Organisatie: %s", org))
	}
	if dialect := strings.TrimSpace(schema.Dialect); dialect != "" {
		parts = append(parts, fmt.Sprintf("Dialect: %s", labelWithCode(dialect, models.DialectLabels)))
	}
	if rootType := strings.TrimSpace(schema.RootType); rootType != "" {
		parts = append(parts, fmt.Sprintf("Root type: %s", labelWithCode(rootType, models.RootTypeLabels)))
	}
	if collection := strings.TrimSpace(schema.Collection); collection != "" {
		parts = append(parts, fmt.Sprintf("Collectie: %s", collection))
	}

	if len(parts) == 0 {
		return schemaTitle(schema)
	}
	return strings.Join(parts, "\n\n")
}

func buildTags(cfg config, schema *models.Schema) []string {
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

func labelWithCode(value string, labels map[string][2]string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if meta, ok := labels[value]; ok {
		return fmt.Sprintf("%s (%s)", meta[0], value)
	}
	return value
}
