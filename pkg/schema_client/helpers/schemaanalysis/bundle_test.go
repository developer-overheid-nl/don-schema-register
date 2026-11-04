package schemaanalysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mapLookup(m map[string]map[string]any) Lookup {
	return func(uri string) (map[string]any, bool) {
		c, ok := m[uri]
		return c, ok
	}
}

func TestBundleEmbedsTransitiveDependencies(t *testing.T) {
	a := map[string]any{"$ref": "https://x.nl/b.json"}
	b := map[string]any{"properties": map[string]any{"c": map[string]any{"$ref": "c.json"}}}
	c := map[string]any{"type": "string"}
	lookup := mapLookup(map[string]map[string]any{
		"https://x.nl/b.json": b,
		"https://x.nl/c.json": c,
	})

	bundle, unresolved := Bundle(a, "https://x.nl/a.json", lookup)

	assert.Empty(t, unresolved)
	assert.Equal(t, "https://x.nl/b.json", a["$ref"])
	assert.Equal(t, "https://x.nl/b.json", bundle["$ref"])
	defs, ok := bundle["$defs"].(map[string]any)
	require.True(t, ok)
	require.Len(t, defs, 2)
	for _, uri := range []string{"https://x.nl/b.json", "https://x.nl/c.json"} {
		d, ok := defs[uri].(map[string]any)
		require.True(t, ok, uri)
		assert.Equal(t, uri, d["$id"])
	}
}

func TestBundleHandlesCycles(t *testing.T) {
	a := map[string]any{"$id": "https://x.nl/a.json", "$ref": "https://x.nl/b.json"}
	b := map[string]any{"$ref": "https://x.nl/a.json"}
	lookup := mapLookup(map[string]map[string]any{
		"https://x.nl/a.json": a,
		"https://x.nl/b.json": b,
	})

	bundle, unresolved := Bundle(a, "https://x.nl/a.json", lookup)

	assert.Empty(t, unresolved)
	defs := bundle["$defs"].(map[string]any)
	assert.Len(t, defs, 1)
	assert.Contains(t, defs, "https://x.nl/b.json")
	assert.NotContains(t, defs, "https://x.nl/a.json")
}

func TestBundleUsesDefinitionsForDraft07(t *testing.T) {
	a := map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"$ref":    "https://x.nl/b.json",
	}
	lookup := mapLookup(map[string]map[string]any{"https://x.nl/b.json": {"type": "object"}})

	bundle, _ := Bundle(a, "https://x.nl/a.json", lookup)

	assert.NotContains(t, bundle, "$defs")
	defs := bundle["definitions"].(map[string]any)
	assert.Contains(t, defs, "https://x.nl/b.json")
}

func TestBundleReportsUnresolved(t *testing.T) {
	a := map[string]any{"allOf": []any{
		map[string]any{"$ref": "https://x.nl/onbekend.json"},
		map[string]any{"$ref": "rel.json"},
	}}

	bundle, unresolved := Bundle(a, "", mapLookup(nil))

	require.Len(t, unresolved, 2)
	assert.Equal(t, "https://x.nl/onbekend.json", unresolved[0].Raw)
	assert.Equal(t, "rel.json", unresolved[1].Raw)
	assert.NotContains(t, bundle, "$defs")
	assert.NotContains(t, bundle, "definitions")
}

func TestBundleDoesNotMutateInput(t *testing.T) {
	a := map[string]any{"properties": map[string]any{"p": map[string]any{"$ref": "https://x.nl/b.json"}}}
	lookup := mapLookup(map[string]map[string]any{"https://x.nl/b.json": {"type": "object"}})

	bundle, _ := Bundle(a, "https://x.nl/a.json", lookup)

	assert.NotContains(t, a, "$defs")
	assert.Contains(t, bundle, "$defs")
	bundle["properties"].(map[string]any)["p"] = nil
	assert.NotNil(t, a["properties"].(map[string]any)["p"])
}
