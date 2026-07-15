package util

import (
	"testing"
	"time"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

func TestToSchemaSummaryAndDetail(t *testing.T) {
	createdAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	lastCrawledAt := createdAt.Add(time.Hour)
	orgURI := "https://example.org/org"
	schema := &models.Schema{
		Id:             "schema-1",
		SchemaUrl:      "https://example.org/schema.json",
		Title:          "Bier",
		Description:    "Een bier",
		Dialect:        "2020-12",
		RootType:       "object",
		Collection:     "demo",
		ContactName:    "Team",
		ContactUrl:     "https://example.org/contact",
		ContactEmail:   "team@example.org",
		OrganisationID: &orgURI,
		Organisation:   &models.Organisation{Uri: orgURI, Label: "Example Org"},
		Content:        map[string]any{"type": "object"},
		CreatedAt:      createdAt,
		LastCrawledAt:  lastCrawledAt,
	}

	summary := ToSchemaSummary(schema)
	if summary.Id != schema.Id || summary.Title != schema.Title || summary.SchemaUrl != schema.SchemaUrl {
		t.Fatalf("summary = %#v, want schema fields copied", summary)
	}
	if summary.Contact.Name != "Team" || summary.Contact.URL != "https://example.org/contact" || summary.Contact.Email != "team@example.org" {
		t.Fatalf("contact = %#v, want contact fields copied", summary.Contact)
	}
	if summary.Organisation == nil || summary.Organisation.Uri != orgURI || summary.Organisation.Label != "Example Org" {
		t.Fatalf("organisation = %#v, want summary organisation", summary.Organisation)
	}
	if !summary.CreatedAt.Equal(createdAt) || !summary.LastCrawledAt.Equal(lastCrawledAt) {
		t.Fatalf("timestamps = %s/%s, want %s/%s", summary.CreatedAt, summary.LastCrawledAt, createdAt, lastCrawledAt)
	}

	detail := ToSchemaDetail(schema)
	if detail.Id != schema.Id {
		t.Fatalf("detail id = %q, want %q", detail.Id, schema.Id)
	}
	if detail.Content["type"] != "object" {
		t.Fatalf("detail content = %#v, want schema content", detail.Content)
	}
}

func TestToSchemaSummaryWithoutOrganisation(t *testing.T) {
	summary := ToSchemaSummary(&models.Schema{Id: "schema-1"})
	if summary.Organisation != nil {
		t.Fatalf("organisation = %#v, want nil", summary.Organisation)
	}
}
