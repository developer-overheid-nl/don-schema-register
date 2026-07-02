package services

import (
	"strings"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	commonfilters "github.com/developer-overheid-nl/don-register-common/filters"
)

func buildDialectGroup(p *models.SchemaFiltersParams, counts *models.SchemaFilterCounts) models.FilterGroup {
	return models.FilterGroup{
		Key:         "dialect",
		Label:       "JSON Schema dialect",
		Description: "Het JSON Schema dialect zoals opgegeven in $schema.",
		Type:        "multi-select",
		Options:     buildLabeledOptions(counts.Dialect, selectedSet(p.Dialect), models.DialectLabels),
	}
}

func buildRootTypeGroup(p *models.SchemaFiltersParams, counts *models.SchemaFilterCounts) models.FilterGroup {
	return models.FilterGroup{
		Key:         "rootType",
		Label:       "Root type",
		Description: "Het root type van het schema (type op het hoogste niveau).",
		Type:        "multi-select",
		Options:     buildLabeledOptions(counts.RootType, selectedSet(p.RootType), models.RootTypeLabels),
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
	options = appendMissingSelectedOptions(options, selected, func(value string) models.FilterOption {
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

func buildLabeledOptions(counts []models.FilterCount, selected map[string]bool, labels map[string][2]string) []models.FilterOption {
	return commonfilters.LabeledOptions(counts, selected, labels, true)
}

func appendMissingSelectedOptions(options []models.FilterOption, selected map[string]bool, build func(string) models.FilterOption) []models.FilterOption {
	return commonfilters.AppendMissingSelectedOptions(options, selected, build)
}

func selectedSet(groups ...[]string) map[string]bool {
	return commonfilters.SelectedSet(groups...)
}
