package schemaanalysis

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/jsonschema"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

// Health beoordeelt een schema met een eigen, eenvoudige regelset en geeft
// een score (0-100) en de gevonden issues, in de volgorde van de regels.
// unresolved zijn de refs die niet binnen het register te vinden zijn.
func Health(content map[string]any, schemaURL string, unresolved []Ref) (int, []models.HealthIssue) {
	var issues []models.HealthIssue
	total, passed := 0, 0

	// check registreert één regel; pointers is leeg als er geen issue is.
	check := func(name, message string, weight int, pointers []string) {
		total += weight
		if len(pointers) == 0 {
			passed += weight
			return
		}
		issues = append(issues, models.HealthIssue{
			Name: name, Message: message, Pointers: pointers,
		})
	}
	root := []string{""}
	onRoot := func(bad bool) []string {
		if bad {
			return root
		}
		return nil
	}

	dialect, _ := content["$schema"].(string)
	check("schema_dialect", "$schema ontbreekt of is onbekend", 3,
		onRoot(dialect == "" || jsonschema.NormalizeDialect(dialect) == "unknown"))

	if schemaURL != "" {
		id, hasID := content["$id"].(string)
		var p []string
		switch {
		case !hasID || id == "":
			p = root
		case id != schemaURL:
			p = []string{"/$id"}
		}
		check("id_matches_url", "$id komt niet overeen met de URL van het schema", 3, p)
	}

	check("top_level_title", "title ontbreekt", 2, onRoot(isBlank(content["title"])))
	check("top_level_description", "description ontbreekt", 2, onRoot(isBlank(content["description"])))
	examples, _ := content["examples"].([]any)
	check("top_level_examples", "examples ontbreekt of is leeg", 1, onRoot(len(examples) == 0))

	var missing []string
	collectUndescribed(content, "", &missing)
	sort.Strings(missing)
	check("property_descriptions", "properties zonder description", 2, missing)

	var at []string
	for _, r := range unresolved {
		at = append(at, r.At)
	}
	check("unresolved_refs", "$refs verwijzen naar schemas buiten het register", 3, at)

	if total == 0 {
		return 100, issues
	}
	return int(math.Round(100 * float64(passed) / float64(total))), issues
}

func isBlank(v any) bool {
	s, _ := v.(string)
	return strings.TrimSpace(s) == ""
}

// collectUndescribed loopt recursief door subschema's en verzamelt de
// pointers van properties zonder description en zonder $ref.
func collectUndescribed(schema map[string]any, ptr string, out *[]string) {
	if props, ok := schema["properties"].(map[string]any); ok {
		for name, v := range props {
			p := ptr + "/properties/" + pointerEscaper.Replace(name)
			sub, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if _, hasRef := sub["$ref"]; !hasRef && isBlank(sub["description"]) {
				*out = append(*out, p)
			}
			collectUndescribed(sub, p, out)
		}
	}
	switch items := schema["items"].(type) {
	case map[string]any:
		collectUndescribed(items, ptr+"/items", out)
	case []any:
		for i, v := range items {
			if sub, ok := v.(map[string]any); ok {
				collectUndescribed(sub, ptr+"/items/"+strconv.Itoa(i), out)
			}
		}
	}
	for _, key := range []string{"$defs", "definitions"} {
		if defs, ok := schema[key].(map[string]any); ok {
			for name, v := range defs {
				if sub, ok := v.(map[string]any); ok {
					collectUndescribed(sub, ptr+"/"+key+"/"+pointerEscaper.Replace(name), out)
				}
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		if list, ok := schema[key].([]any); ok {
			for i, v := range list {
				if sub, ok := v.(map[string]any); ok {
					collectUndescribed(sub, ptr+"/"+key+"/"+strconv.Itoa(i), out)
				}
			}
		}
	}
}
