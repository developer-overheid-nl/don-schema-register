package schemaanalysis

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyzeApiSchema(t *testing.T) {
	const base = "https://static.developer.overheid.nl/schemas/don/"
	const dialect = "https://json-schema.org/draft/2020-12/schema"
	api := map[string]any{
		"$schema": dialect,
		"$id":     base + "Api.schema.json",
		"title":   "Api",
		"type":    "object",
		"properties": map[string]any{
			"organisation": map[string]any{"$ref": "Organisation.schema.json"},
			"lifecycle":    map[string]any{"$ref": "ApiLifecycle.schema.json"},
		},
	}
	org := map[string]any{
		"$schema": dialect, "$id": base + "Organisation.schema.json",
		"title": "Organisation", "type": "object",
		"properties": map[string]any{"label": map[string]any{"type": "string", "description": "Naam"}},
	}
	life := map[string]any{
		"$schema": dialect, "$id": base + "ApiLifecycle.schema.json",
		"title": "ApiLifecycle", "type": "string", "enum": []any{"draft", "live"},
	}
	lookup := func(uri string) (map[string]any, bool) {
		switch uri {
		case base + "Organisation.schema.json":
			return org, true
		case base + "ApiLifecycle.schema.json":
			return life, true
		}
		return nil, false
	}
	raw, err := json.MarshalIndent(api, "", "  ")
	require.NoError(t, err)

	res := Analyze(api, raw, base+"Api.schema.json", lookup)

	require.Len(t, res.Dependencies, 2)
	for _, d := range res.Dependencies {
		assert.Equal(t, base+"Api.schema.json", d.From)
	}
	tos := map[string]string{}
	for _, d := range res.Dependencies {
		tos[d.At] = d.To
	}
	assert.Equal(t, base+"Organisation.schema.json", tos["/properties/organisation/$ref"])
	assert.Equal(t, base+"ApiLifecycle.schema.json", tos["/properties/lifecycle/$ref"])
	assert.Equal(t, 2, res.DependencyCount)
	assert.Equal(t, len(raw), res.Bytes)
	assert.Greater(t, res.BytesBundled, res.Bytes)
	bundle, _ := Bundle(api, base+"Api.schema.json", lookup)
	bb, _ := json.MarshalIndent(bundle, "", "  ")
	assert.Equal(t, len(bb), res.BytesBundled)
	assert.Equal(t, dialect, res.Dialect)
	assert.Equal(t, dialect, res.BaseDialect)
	for _, i := range res.HealthIssues {
		assert.NotEqual(t, "unresolved_refs", i.Name)
	}
}

func TestAnalyzeUnresolvedAndEmptyRaw(t *testing.T) {
	content := map[string]any{
		"$id":        "https://example.com/a.json",
		"properties": map[string]any{"x": map[string]any{"$ref": "missing.json"}},
	}
	res := Analyze(content, nil, "https://example.com/a.json", func(string) (map[string]any, bool) { return nil, false })
	require.Len(t, res.Dependencies, 1)
	assert.Equal(t, "https://example.com/missing.json", res.Dependencies[0].To)
	assert.Equal(t, 1, res.DependencyCount)
	b, _ := json.MarshalIndent(content, "", "  ")
	assert.Equal(t, len(b), res.Bytes)
	found := false
	for _, i := range res.HealthIssues {
		found = found || i.Name == "unresolved_refs"
	}
	assert.True(t, found)
}

func TestAnalyzeIgnoresUnresolvedRefsOfDependencies(t *testing.T) {
	const base = "https://schemas.example.org/"
	a := map[string]any{
		"$id": base + "A", "title": "A", "type": "object",
		"properties": map[string]any{"b": map[string]any{"$ref": base + "B"}},
	}
	b := map[string]any{
		"$id": base + "B", "title": "B", "type": "object",
		"properties": map[string]any{"x": map[string]any{"$ref": base + "Onbekend"}},
	}
	lookup := func(uri string) (map[string]any, bool) {
		if uri == base+"B" {
			return b, true
		}
		return nil, false
	}
	hasUnresolved := func(res Result) bool {
		for _, i := range res.HealthIssues {
			if i.Name == "unresolved_refs" {
				return true
			}
		}
		return false
	}

	// Een onoplosbare ref in een dependency is niet de schuld van A.
	assert.False(t, hasUnresolved(Analyze(a, nil, base+"A", lookup)))
	// B zelf heeft de onoplosbare ref wel.
	assert.True(t, hasUnresolved(Analyze(b, nil, base+"B", lookup)))

	// A met een eigen onoplosbare ref blijft de issue houden.
	a["properties"].(map[string]any)["y"] = map[string]any{"$ref": base + "Weg"}
	assert.True(t, hasUnresolved(Analyze(a, nil, base+"A", lookup)))
}
