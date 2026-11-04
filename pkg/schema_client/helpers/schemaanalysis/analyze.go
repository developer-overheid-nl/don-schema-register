package schemaanalysis

import (
	"encoding/json"
	"strings"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

// Result is de uitkomst van de analyse van één schema.
type Result struct {
	Dependencies    []models.SourceMetaDependency
	DependencyCount int
	Health          int
	HealthIssues    []models.SourceMetaHealthIssue
	Bytes           int
	BytesBundled    int
	BaseDialect     string
	Dialect         string
}

// Analyze combineert refs, bundel en health tot één resultaat. raw is de
// originele inhoud zoals geserveerd (Bytes = len(raw)); is die leeg, dan wordt
// de lengte van de ingesprongen marshal van content gebruikt. BytesBundled is
// de lengte van de ingesprongen bundel, zoals de bundle-endpoint die serveert.
func Analyze(content map[string]any, raw []byte, schemaURL string, lookup Lookup) Result {
	base := BaseURI(content, schemaURL)

	var res Result
	unique := map[string]bool{}
	// Health krijgt alleen de onoplosbare refs van het root-schema: de At van
	// refs in ingebedde dependencies is relatief aan die dependency.
	var unresolved []Ref
	for _, ref := range ExternalRefs(content, base) {
		to := ref.Target
		if to == "" {
			to = ref.Raw
			unresolved = append(unresolved, ref)
		} else {
			unique[to] = true
			if _, ok := lookup(to); !ok && to != base {
				unresolved = append(unresolved, ref)
			}
		}
		res.Dependencies = append(res.Dependencies, models.SourceMetaDependency{
			From: base, To: to, At: ref.At,
		})
	}
	res.DependencyCount = len(unique)

	bundle, _ := Bundle(content, base, lookup)
	res.Health, res.HealthIssues = Health(content, schemaURL, unresolved)

	if len(raw) > 0 {
		res.Bytes = len(raw)
	} else if b, err := json.MarshalIndent(content, "", "  "); err == nil {
		res.Bytes = len(b)
	}
	if b, err := json.MarshalIndent(bundle, "", "  "); err == nil {
		res.BytesBundled = len(b)
	}

	dialect, _ := content["$schema"].(string)
	dialect = strings.TrimSpace(dialect)
	res.BaseDialect, res.Dialect = dialect, dialect
	return res
}
