package services

import (
	"strings"

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
		Label:       "Root type",
		Description: "Het root type van het schema (type op het hoogste niveau).",
		Type:        "multi-select",
		Options:     commonfilters.LabeledOptions(counts.RootType, commonfilters.SelectedSet(p.RootType), models.RootTypeLabels, true),
	}
}

func buildOrganisationGroup(p *models.SchemaFiltersParams, counts *models.SchemaFilterCounts) models.FilterGroup {
	selected := map[string]bool{}
	if p != nil && p.Organisation != nil {
		if value := strings.TrimSpace(*p.Organisation); value != "" {
			selected[value] = true
		}
	}

	options := make([]models.FilterOption, 0, len(counts.Organisation))
	for _, fc := range counts.Organisation {
		label := fc.Label
		if label == "" {
			label = fc.Value
		}
		options = append(options, models.FilterOption{
			Value:    fc.Value,
			Label:    label,
			Count:    fc.Count,
			Selected: selected[fc.Value],
		})
	}
	options = commonfilters.AppendMissingSelectedOptions(options, selected, func(value string) models.FilterOption {
		return models.FilterOption{
			Value:    value,
			Label:    value,
			Selected: true,
		}
	})
	commonfilters.SortOptions(options)

	return models.FilterGroup{
		Key:         "organisation",
		Label:       "Organisatie",
		Description: "De overheidsorganisatie die het schema beheert.",
		Type:        "single-select",
		Options:     options,
	}
}
