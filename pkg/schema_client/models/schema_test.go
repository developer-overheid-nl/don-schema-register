package models

import "testing"

func TestListSchemasParamsSchemaFiltersCopiesValues(t *testing.T) {
	org := "https://example.org/org"
	params := &ListSchemasParams{
		Organisation: &org,
		Query:        "bier",
		Dialect:      []string{"2020-12"},
		RootType:     []string{"object"},
	}

	filters := params.SchemaFilters()
	if filters.Organisation != nil {
		t.Fatalf("organisation = %#v, want nil", filters.Organisation)
	}
	if filters.Query != "bier" || filters.Dialect[0] != "2020-12" || filters.RootType[0] != "object" {
		t.Fatalf("filters = %#v, want copied fields", filters)
	}

	params.Dialect[0] = "changed"
	params.RootType[0] = "changed"
	if filters.Dialect[0] != "2020-12" || filters.RootType[0] != "object" {
		t.Fatalf("filters share slices with params: %#v", filters)
	}
}

func TestNilListSchemasParamsSchemaFilters(t *testing.T) {
	var params *ListSchemasParams
	filters := params.SchemaFilters()
	if filters == nil {
		t.Fatal("filters = nil, want empty filters")
	}
}
