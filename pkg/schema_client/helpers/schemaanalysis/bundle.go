package schemaanalysis

import "strings"

// Lookup zoekt een schema op absolute URI zonder fragment.
type Lookup func(uri string) (map[string]any, bool)

// Bundle maakt een compound document: een diepe kopie van content waarin elk
// transitief bereikbaar schema eenmaal is ingevoegd onder $defs (of
// definitions bij draft-04/06/07), met zijn absolute URI als key. $ref-waarden
// blijven ongewijzigd. unresolved bevat refs zonder bepaalbare of vindbare
// target.
func Bundle(content map[string]any, baseURI string, lookup Lookup) (map[string]any, []Ref) {
	bundle := deepCopyMap(content)
	rootURI := BaseURI(bundle, baseURI)

	defsKey := "$defs"
	if s, _ := bundle["$schema"].(string); strings.Contains(s, "draft-04") ||
		strings.Contains(s, "draft-06") || strings.Contains(s, "draft-07") {
		defsKey = "definitions"
	}

	var unresolved []Ref
	visited := map[string]bool{}
	if rootURI != "" {
		visited[rootURI] = true
	}
	type item struct {
		doc  map[string]any
		base string
	}
	queue := []item{{bundle, rootURI}}
	var embedded map[string]any

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, ref := range ExternalRefs(cur.doc, cur.base) {
			if ref.Target == "" {
				unresolved = append(unresolved, ref)
				continue
			}
			if visited[ref.Target] {
				continue
			}
			found, ok := lookup(ref.Target)
			if !ok {
				unresolved = append(unresolved, ref)
				continue
			}
			visited[ref.Target] = true
			dep := deepCopyMap(found)
			if BaseURI(dep, "") == "" {
				dep["$id"] = ref.Target
			}
			if embedded == nil {
				if m, ok := bundle[defsKey].(map[string]any); ok {
					embedded = m
				} else {
					embedded = map[string]any{}
					bundle[defsKey] = embedded
				}
			}
			embedded[ref.Target] = dep
			queue = append(queue, item{dep, BaseURI(dep, ref.Target)})
		}
	}
	return bundle, unresolved
}

func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = deepCopy(v)
	}
	return out
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyMap(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = deepCopy(e)
		}
		return out
	default:
		return v
	}
}
