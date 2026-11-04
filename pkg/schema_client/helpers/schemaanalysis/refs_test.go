package schemaanalysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBase = "https://static.developer.overheid.nl/schemas/don/Api.schema.json"

func TestExternalRefsResolvesRelativeToBase(t *testing.T) {
	content := map[string]any{
		"properties": map[string]any{
			"lifecycle":    map[string]any{"$ref": "ApiLifecycle.schema.json"},
			"organisation": map[string]any{"$ref": "Organisation.schema.json#/$defs/x"},
		},
	}
	refs := ExternalRefs(content, testBase)
	require.Len(t, refs, 2)
	assert.Equal(t, "/properties/lifecycle/$ref", refs[0].At)
	assert.Equal(t, "https://static.developer.overheid.nl/schemas/don/ApiLifecycle.schema.json", refs[0].Target)
	assert.Equal(t, "/properties/organisation/$ref", refs[1].At)
	assert.Equal(t, "Organisation.schema.json#/$defs/x", refs[1].Raw)
	assert.Equal(t, "https://static.developer.overheid.nl/schemas/don/Organisation.schema.json", refs[1].Target)
}

func TestExternalRefsSkipsLocalRefs(t *testing.T) {
	content := map[string]any{"a": map[string]any{"$ref": "#/$defs/a"}}
	assert.Empty(t, ExternalRefs(content, testBase))
}

func TestExternalRefsWithoutBaseLeavesTargetEmpty(t *testing.T) {
	content := map[string]any{
		"a": map[string]any{"$ref": "Other.schema.json"},
		"b": map[string]any{"$ref": "https://x.nl/a.json"},
	}
	refs := ExternalRefs(content, "")
	require.Len(t, refs, 2)
	assert.Equal(t, "", refs[0].Target)
	assert.Equal(t, "https://x.nl/a.json", refs[1].Target)
}

func TestExternalRefsWalksArraysAndEscapesPointer(t *testing.T) {
	content := map[string]any{
		"allOf": []any{
			map[string]any{"type": "object"},
			map[string]any{"$ref": "B.json"},
		},
		"properties": map[string]any{"a/b": map[string]any{"$ref": "C.json"}},
	}
	refs := ExternalRefs(content, testBase)
	require.Len(t, refs, 2)
	assert.Equal(t, "/allOf/1/$ref", refs[0].At)
	assert.Equal(t, "/properties/a~1b/$ref", refs[1].At)
}

func TestCompactBytes(t *testing.T) {
	assert.Equal(t, 7, CompactBytes([]byte("{ \"a\": 1 }")))
	assert.Equal(t, 3, CompactBytes([]byte("{{{")))
}

func TestBaseURI(t *testing.T) {
	assert.Equal(t, "https://x.nl/a.json", BaseURI(map[string]any{"$id": "https://x.nl/a.json"}, "fb"))
	assert.Equal(t, "fb", BaseURI(map[string]any{"$id": "a.json"}, "fb"))
	assert.Equal(t, "fb", BaseURI(map[string]any{}, "fb"))
}
