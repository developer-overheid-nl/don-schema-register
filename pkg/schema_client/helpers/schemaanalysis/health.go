package schemaanalysis

import (
	_ "embed"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/jsonschema"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

//go:embed health-rules.json
var healthRulesJSON []byte

// healthRule is een regel uit health-rules.json.
type healthRule struct {
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
	Message     string `json:"message"`
	Description string `json:"description"`
}

// healthInput bundelt wat de regels nodig hebben.
type healthInput struct {
	content    map[string]any
	schemaURL  string
	unresolved []Ref
}

// healthCheck geeft de pointers van een overtreding (leeg als de regel
// slaagt) en of de regel van toepassing is.
type healthCheck func(in healthInput) (pointers []string, applicable bool)

var rootPointer = []string{""}

func onRoot(bad bool) []string {
	if bad {
		return rootPointer
	}
	return nil
}

// healthChecks bevat per regelnaam de implementatie; gewicht, melding en
// uitleg staan in health-rules.json.
var healthChecks = map[string]healthCheck{
	"schema_dialect": func(in healthInput) ([]string, bool) {
		dialect, _ := in.content["$schema"].(string)
		return onRoot(dialect == "" || jsonschema.NormalizeDialect(dialect) == "unknown"), true
	},
	"id_matches_url": func(in healthInput) ([]string, bool) {
		if in.schemaURL == "" {
			return nil, false
		}
		id, hasID := in.content["$id"].(string)
		switch {
		case !hasID || id == "":
			return rootPointer, true
		case id != in.schemaURL:
			return []string{"/$id"}, true
		}
		return nil, true
	},
	"top_level_title": func(in healthInput) ([]string, bool) {
		return onRoot(isBlank(in.content["title"])), true
	},
	"top_level_description": func(in healthInput) ([]string, bool) {
		return onRoot(isBlank(in.content["description"])), true
	},
	"top_level_examples": func(in healthInput) ([]string, bool) {
		examples, _ := in.content["examples"].([]any)
		return onRoot(len(examples) == 0), true
	},
	"property_descriptions": func(in healthInput) ([]string, bool) {
		var missing []string
		collectUndescribed(in.content, "", &missing)
		sort.Strings(missing)
		return missing, true
	},
	"unresolved_refs": func(in healthInput) ([]string, bool) {
		var at []string
		for _, r := range in.unresolved {
			at = append(at, r.At)
		}
		return at, true
	},
}

var (
	healthRulesOnce sync.Once
	healthRules     []healthRule
)

// loadHealthRules parst de ingebedde regels één keer; ongeldige JSON is een
// buildfout en leidt tot een panic.
func loadHealthRules() []healthRule {
	healthRulesOnce.Do(func() {
		if err := json.Unmarshal(healthRulesJSON, &healthRules); err != nil {
			panic("schemaanalysis: ongeldige health-rules.json: " + err.Error())
		}
	})
	return healthRules
}

// Health beoordeelt een schema met de regels uit health-rules.json en geeft
// een score (0-100) en de gevonden issues, in de volgorde van het bestand.
// unresolved zijn de refs die niet binnen het register te vinden zijn.
func Health(content map[string]any, schemaURL string, unresolved []Ref) (int, []models.HealthIssue) {
	var issues []models.HealthIssue
	total, passed := 0, 0
	in := healthInput{content: content, schemaURL: schemaURL, unresolved: unresolved}

	for _, rule := range loadHealthRules() {
		fn, ok := healthChecks[rule.Name]
		if !ok {
			continue
		}
		pointers, applicable := fn(in)
		if !applicable {
			continue
		}
		total += rule.Weight
		if len(pointers) == 0 {
			passed += rule.Weight
			continue
		}
		issues = append(issues, models.HealthIssue{
			Name: rule.Name, Message: rule.Message,
			Description: rule.Description, Pointers: pointers,
		})
	}

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
