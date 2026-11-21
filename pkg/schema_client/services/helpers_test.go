package services

import (
	"errors"
	"net/http"
	"testing"

	problem "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/problem"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

func TestErrorDetailHelpers(t *testing.T) {
	tests := []struct {
		name   string
		detail problem.ErrorDetail
		in     string
		loc    string
	}{
		{name: "body", detail: bodyError("schemaUrl", "required", "is required"), in: "body", loc: "#/schemaUrl"},
		{name: "path", detail: pathError("id", "required", "is required"), in: "path", loc: "#/id"},
		{name: "query", detail: queryError("q", "required", "is required"), in: "query", loc: "#/q"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.detail.In != tt.in || tt.detail.Location != tt.loc {
				t.Fatalf("detail = %#v, want %s %s", tt.detail, tt.in, tt.loc)
			}
		})
	}
}

func TestValidateSchemaID(t *testing.T) {
	if err := validateSchemaID("schema-1"); err != nil {
		t.Fatalf("validateSchemaID(valid) error = %v", err)
	}

	err := validateSchemaID("")
	var p problem.ProblemJSON
	if !errors.As(err, &p) || p.Status != http.StatusBadRequest {
		t.Fatalf("error = %v, want bad request problem", err)
	}

	err = validateSchemaID("bad\x00id")
	if !errors.As(err, &p) || p.Status != http.StatusBadRequest {
		t.Fatalf("error = %v, want bad request problem", err)
	}
}

func TestDeriveOrganisationURI(t *testing.T) {
	if got := deriveOrganisationURI(nil); got != "" {
		t.Fatalf("deriveOrganisationURI(nil) = %q, want empty", got)
	}
	if got := deriveOrganisationURI(&models.Schema{Organisation: &models.Organisation{Uri: " https://example.org/org "}}); got != "https://example.org/org" {
		t.Fatalf("deriveOrganisationURI(org) = %q", got)
	}
	orgID := " https://example.org/id "
	if got := deriveOrganisationURI(&models.Schema{OrganisationID: &orgID}); got != "https://example.org/id" {
		t.Fatalf("deriveOrganisationURI(id) = %q", got)
	}
}
