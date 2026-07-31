package jsonschema

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/stretchr/testify/require"
)

func TestNormalizeDialect(t *testing.T) {
	cases := map[string]string{
		"https://json-schema.org/draft/2020-12/schema":   "2020-12",
		"http://json-schema.org/draft/2020-12/schema#":   "2020-12",
		"https://json-schema.org/draft/2019-09/schema":   "2019-09",
		"http://json-schema.org/draft-07/schema#":        "draft-07",
		"http://json-schema.org/draft-06/schema#":        "draft-06",
		"http://json-schema.org/draft-04/schema#":        "draft-04",
		"https://spec.openapis.org/oas/3.1/dialect/base": "oas-3.1",
		"":                                       "unknown",
		"https://example.org/my-own-meta-schema": "unknown",
		"HTTPS://JSON-SCHEMA.ORG/DRAFT/2020-12/SCHEMA":  "2020-12",
		"https://json-schema.org/draft/2020-12/schema/": "2020-12",
	}
	for input, expected := range cases {
		require.Equal(t, expected, NormalizeDialect(input), "input: %q", input)
	}
}

func TestRootType(t *testing.T) {
	require.Equal(t, "object", RootType(map[string]any{"type": "object"}))
	require.Equal(t, "array", RootType(map[string]any{"type": "array"}))
	require.Equal(t, "string", RootType(map[string]any{"type": []any{"string", "null"}}))
	require.Equal(t, "unknown", RootType(map[string]any{"allOf": []any{}}))
	require.Equal(t, "unknown", RootType(map[string]any{}))
	require.Equal(t, "unknown", RootType(map[string]any{"type": ""}))
}

func TestParseValidateAndHash(t *testing.T) {
	raw := []byte(`{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"title": "Bier",
		"description": "Een bier",
		"type": "object"
	}`)
	res, err := ParseValidateAndHash(raw, "application/schema+json")
	require.NoError(t, err)
	require.Equal(t, "2020-12", res.Dialect)
	require.Equal(t, "object", res.RootType)
	require.Equal(t, "Bier", res.Content["title"])
	require.NotEmpty(t, res.Hash)

	// hash is deterministisch, onafhankelijk van whitespace
	raw2 := []byte(`{"title":"Bier","description":"Een bier","type":"object","$schema":"https://json-schema.org/draft/2020-12/schema"}`)
	res2, err := ParseValidateAndHash(raw2, "")
	require.NoError(t, err)
	require.Equal(t, res.Hash, res2.Hash)
}

func TestParseValidateAndHashInvalidJSON(t *testing.T) {
	_, err := ParseValidateAndHash([]byte(`{not json`), "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid JSON Schema")
}

func TestParseValidateAndHashRejectsNonObject(t *testing.T) {
	_, err := ParseValidateAndHash([]byte(`"gewoon een string"`), "")
	require.Error(t, err)
}

func TestFetchParseValidateAndHashFromURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/schema+json")
		_, _ = w.Write([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Remote","type":"object"}`))
	}))
	defer server.Close()

	res, err := FetchParseValidateAndHash(context.Background(), SchemaInput{SchemaUrl: server.URL}, FetchOpts{HTTPClient: server.Client()})
	require.NoError(t, err)
	require.Equal(t, "Remote", res.Content["title"])
	require.Equal(t, "2020-12", res.Dialect)
}

func TestFetchParseValidateAndHashHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := FetchParseValidateAndHash(context.Background(), SchemaInput{SchemaUrl: server.URL}, FetchOpts{HTTPClient: server.Client()})
	require.Error(t, err)
	require.True(t, IsHTTPStatus(err, http.StatusNotFound))
}

func TestFetchParseValidateAndHashEmptyInput(t *testing.T) {
	_, err := FetchParseValidateAndHash(context.Background(), SchemaInput{}, FetchOpts{})
	require.Error(t, err)
}

func TestBuildSchemaAndValidate(t *testing.T) {
	res, err := ParseValidateAndHash([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","title":"Bier","description":"Een bier","type":"object"}`), "")
	require.NoError(t, err)

	request := models.SchemaPost{
		SchemaUrl:       "https://example.org/bier.schema.json",
		OrganisationUri: "https://example.org/org",
		Contact: models.Contact{
			Name:  "Team",
			Email: "team@example.org",
			URL:   "https://example.org",
		},
	}
	schema := BuildSchema(res, request, "Example Org")
	require.NotEmpty(t, schema.Id)
	require.Equal(t, "Bier", schema.Title)
	require.Equal(t, "Een bier", schema.Description)
	require.Equal(t, "2020-12", schema.Dialect)
	require.Equal(t, "object", schema.RootType)
	require.Equal(t, "https://example.org/bier.schema.json", schema.SchemaUrl)
	require.NotNil(t, schema.Organisation)
	require.Equal(t, "Example Org", schema.Organisation.Label)
	require.Empty(t, ValidateSchema(schema))
}

func TestValidateSchemaMissingContact(t *testing.T) {
	res, err := ParseValidateAndHash([]byte(`{"type":"object"}`), "")
	require.NoError(t, err)

	schema := BuildSchema(res, models.SchemaPost{SchemaUrl: "https://example.org/s.json"}, "")
	invalids := ValidateSchema(schema)
	require.Len(t, invalids, 3)
	locations := make([]string, 0, len(invalids))
	for _, invalid := range invalids {
		locations = append(locations, invalid.Location)
	}
	require.Contains(t, locations, "#/contact/name")
	require.Contains(t, locations, "#/contact/email")
	require.Contains(t, locations, "#/contact/url")
}

func TestHTTPStatusErrorError(t *testing.T) {
	err := &HTTPStatusError{StatusCode: 404, Body: "not found"}
	require.Equal(t, "kan schema niet ophalen: status 404: not found", err.Error())
}

func TestUpdateSchemaFromContent(t *testing.T) {
	schema := &models.Schema{Id: "schema-1", Title: "Old"}
	res, err := ParseValidateAndHash([]byte(`{"title":"New","description":"Updated","type":"array"}`), "application/schema+json")
	require.NoError(t, err)

	UpdateSchemaFromContent(schema, res, models.SchemaPost{
		SchemaUrl:       "https://example.org/schema.json",
		OrganisationUri: "https://example.org/org",
		Contact: models.Contact{
			Name:  "Team",
			URL:   "https://example.org/team",
			Email: "team@example.org",
		},
	}, "Example Org")

	require.Equal(t, "New", schema.Title)
	require.Equal(t, "Updated", schema.Description)
	require.Equal(t, "array", schema.RootType)
	require.Equal(t, "https://example.org/schema.json", schema.SchemaUrl)
	require.Equal(t, "Team", schema.ContactName)
	require.NotNil(t, schema.Organisation)
	require.Equal(t, "Example Org", schema.Organisation.Label)
}
