// Package schemaanalysis bevat eigen analyse van JSON Schemas: refs,
// grootte en dialect. Het vervangt de analyse door SourceMeta One.
package schemaanalysis

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Ref beschrijft één externe $ref in een schema.
type Ref struct {
	// At is de JSON-pointer naar de $ref, bv. /properties/lifecycle/$ref.
	At string
	// Raw is de originele waarde van de $ref.
	Raw string
	// Target is de absolute URI zonder fragment; leeg als die niet te
	// bepalen is.
	Target string
}

var pointerEscaper = strings.NewReplacer("~", "~0", "/", "~1")

// ExternalRefs geeft alle $refs die niet met # beginnen, gesorteerd op At.
// Relatieve refs worden geresolved tegen baseURI.
func ExternalRefs(content map[string]any, baseURI string) []Ref {
	var base *url.URL
	if u, err := url.Parse(baseURI); err == nil && u.IsAbs() {
		base = u
	}
	var refs []Ref
	var walk func(node any, path string)
	walk = func(node any, path string) {
		switch v := node.(type) {
		case map[string]any:
			for k, child := range v {
				p := path + "/" + pointerEscaper.Replace(k)
				if s, ok := child.(string); ok && k == "$ref" {
					if !strings.HasPrefix(s, "#") {
						refs = append(refs, Ref{At: p, Raw: s, Target: resolveTarget(s, base)})
					}
					continue
				}
				walk(child, p)
			}
		case []any:
			for i, child := range v {
				walk(child, path+"/"+strconv.Itoa(i))
			}
		}
	}
	walk(content, "")
	sort.Slice(refs, func(i, j int) bool { return refs[i].At < refs[j].At })
	return refs
}

func resolveTarget(raw string, base *url.URL) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if !u.IsAbs() {
		if base == nil {
			return ""
		}
		u = base.ResolveReference(u)
	}
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

// BaseURI geeft $id als die absoluut is, anders fallback.
func BaseURI(content map[string]any, fallback string) string {
	if id, ok := content["$id"].(string); ok {
		if u, err := url.Parse(id); err == nil && u.IsAbs() {
			return id
		}
	}
	return fallback
}
