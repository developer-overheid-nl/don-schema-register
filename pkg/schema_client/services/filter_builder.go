package services

import (
	commonfilters "github.com/developer-overheid-nl/don-register-common/filters"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

func buildDialectGroup(p *models.SchemaFiltersParams, counts *models.SchemaFilterCounts) models.FilterGroup {
	return models.FilterGroup{
		Key:         "dialect",
		Label:       "JSON Schema dialect",
		Description: "Het JSON Schema dialect zoals opgegeven in $schema.",
		Type:        "multi-select",
		Options:     commonfilters.LabeledOptions(counts.Dialect, commonfilters.SelectedSet(p.Dialect), models.DialectLabels, true),
	}
}

func buildRootTypeGroup(p *models.SchemaFiltersParams, counts *models.SchemaFilterCounts) models.FilterGroup {
	return models.FilterGroup{
		Key:         "rootType",
		Label:       "Type",
		Description: "Het root type van het schema (type op het hoogste niveau).",
		Type:        "multi-select",
		Options:     commonfilters.LabeledOptions(counts.RootType, commonfilters.SelectedSet(p.RootType), models.RootTypeLabels, true),
	}
}
