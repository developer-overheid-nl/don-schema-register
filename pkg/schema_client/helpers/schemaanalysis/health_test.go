package schemaanalysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const healthURL = "https://example.org/schemas/Api.schema.json"

func healthyContent() map[string]any {
	return map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"$id":         healthURL,
		"title":       "Api",
		"description": "Een API",
		"examples":    []any{map[string]any{"name": "x"}},
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "Naam"},
			"org":  map[string]any{"$ref": "Org.schema.json"},
		},
	}
}

func TestHealthPerfectSchemaScores100(t *testing.T) {
	score, issues := Health(healthyContent(), healthURL, nil)
	assert.Equal(t, 100, score)
	assert.Empty(t, issues)
}

func TestHealthReportsMissingExamplesAndPropertyDescription(t *testing.T) {
	c := healthyContent()
	delete(c, "examples")
	c["properties"] = map[string]any{"name": map[string]any{"type": "string"}}
	score, issues := Health(c, healthURL, nil)
	assert.Equal(t, 81, score)
	require.Len(t, issues, 2)
	assert.Equal(t, "top_level_examples", issues[0].Name)
	assert.Equal(t, []string{""}, issues[0].Pointers)
	assert.Equal(t, "property_descriptions", issues[1].Name)
	assert.Equal(t, []string{"/properties/name"}, issues[1].Pointers)
}

func TestHealthSkipsIdRuleWithoutURL(t *testing.T) {
	c := healthyContent()
	delete(c, "$id")
	score, issues := Health(c, "", nil)
	assert.Equal(t, 100, score)
	assert.Empty(t, issues)
}

func TestHealthReportsUnresolvedRefs(t *testing.T) {
	unresolved := []Ref{{At: "/properties/x/$ref", Raw: "X.json"}}
	_, issues := Health(healthyContent(), healthURL, unresolved)
	require.Len(t, issues, 1)
	assert.Equal(t, "unresolved_refs", issues[0].Name)
	assert.Equal(t, []string{"/properties/x/$ref"}, issues[0].Pointers)
}

func TestHealthIdMismatchAndDialect(t *testing.T) {
	c := healthyContent()
	c["$id"] = "https://other"
	delete(c, "$schema")
	_, issues := Health(c, healthURL, nil)
	require.Len(t, issues, 2)
	assert.Equal(t, "schema_dialect", issues[0].Name)
	assert.Equal(t, "id_matches_url", issues[1].Name)
	assert.Equal(t, []string{"/$id"}, issues[1].Pointers)
}

func TestHealthNestedPropertyDescriptionsSorted(t *testing.T) {
	c := healthyContent()
	c["properties"] = map[string]any{
		"b/c": map[string]any{"type": "string"},
		"a": map[string]any{"description": "A", "type": "object", "properties": map[string]any{
			"inner": map[string]any{"type": "string"},
		}},
	}
	c["$defs"] = map[string]any{"d": map[string]any{"properties": map[string]any{
		"z": map[string]any{"description": " "},
	}}}
	_, issues := Health(c, healthURL, nil)
	require.Len(t, issues, 1)
	assert.Equal(t, []string{
		"/$defs/d/properties/z",
		"/properties/a/properties/inner",
		"/properties/b~1c",
	}, issues[0].Pointers)
}
