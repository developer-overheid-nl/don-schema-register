// Package jsonschema bevat helpers voor het ophalen, parsen, valideren en
// hashen van JSON Schemas, naar het patroon van helpers/openapi in
// don-api-register.
package jsonschema

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/problem"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/teris-io/shortid"
)

// maxSchemaBytes begrenst de grootte van een op te halen JSON Schema.
const maxSchemaBytes = 10 << 20 // 10 MiB

type SchemaInput struct {
	SchemaUrl  string
	SchemaBody string
}

func (i *SchemaInput) Normalize() {
	i.SchemaUrl = strings.TrimSpace(i.SchemaUrl)
	i.SchemaBody = strings.TrimSpace(i.SchemaBody)
}

func (i SchemaInput) IsEmpty() bool {
	return i.SchemaUrl == "" && i.SchemaBody == ""
}

type FetchOpts struct {
	Origin     string       // bv. "https://developer.overheid.nl"
	HTTPClient *http.Client // optioneel
}

type SchemaResult struct {
	Content     map[string]any // geparste JSON Schema inhoud
	Hash        string         // sha256 van de genormaliseerde inhoud
	Raw         []byte         // oorspronkelijke bytes zoals opgehaald
	ContentType string         // content-type header van de response (kan leeg zijn)
	Dialect     string         // genormaliseerd $schema dialect, bv. "2020-12"
	RootType    string         // root type van het schema, bv. "object"
}

type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("kan schema niet ophalen: status %d: %s", e.StatusCode, strings.TrimSpace(e.Body))
}

func IsHTTPStatus(err error, statusCode int) bool {
	var statusErr *HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == statusCode
}

// FetchParseValidateAndHash haalt het JSON Schema op (of gebruikt de inline
// body), valideert dat het geldig JSON en een plausibel JSON Schema is, en
// berekent een hash over de genormaliseerde inhoud.
func FetchParseValidateAndHash(ctx context.Context, input SchemaInput, opts FetchOpts) (*SchemaResult, error) {
	input.Normalize()
	if input.IsEmpty() {
		return nil, fmt.Errorf("schema input ontbreekt")
	}

	raw, contentType, err := fetchRawSchema(ctx, input, opts)
	if err != nil {
		return nil, err
	}

	return ParseValidateAndHash(raw, contentType)
}

// ParseValidateAndHash parseert en valideert ruwe schema-bytes en berekent de hash.
func ParseValidateAndHash(raw []byte, contentType string) (*SchemaResult, error) {
	var content map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&content); err != nil {
		return nil, fmt.Errorf("invalid JSON Schema (parse): %s", strings.TrimSpace(err.Error()))
	}

	// Hash over een deterministische weergave (json.Marshal sorteert map keys).
	rendered, err := json.Marshal(content)
	if err != nil || len(rendered) == 0 {
		rendered = raw
	}
	sum := sha256.Sum256(rendered)

	return &SchemaResult{
		Content:     content,
		Hash:        hex.EncodeToString(sum[:]),
		Raw:         raw,
		ContentType: contentType,
		Dialect:     NormalizeDialect(stringValue(content, "$schema")),
		RootType:    RootType(content),
	}, nil
}

// NormalizeDialect vertaalt een $schema URI naar een korte dialect-waarde.
func NormalizeDialect(schemaURI string) string {
	uri := strings.ToLower(strings.TrimSpace(schemaURI))
	uri = strings.TrimPrefix(uri, "http://")
	uri = strings.TrimPrefix(uri, "https://")
	uri = strings.TrimSuffix(uri, "#")
	uri = strings.TrimSuffix(uri, "/")
	switch {
	case uri == "":
		return "unknown"
	case strings.HasPrefix(uri, "json-schema.org/draft/2020-12"):
		return "2020-12"
	case strings.HasPrefix(uri, "json-schema.org/draft/2019-09"):
		return "2019-09"
	case strings.HasPrefix(uri, "json-schema.org/draft-07"):
		return "draft-07"
	case strings.HasPrefix(uri, "json-schema.org/draft-06"):
		return "draft-06"
	case strings.HasPrefix(uri, "json-schema.org/draft-04"):
		return "draft-04"
	case strings.HasPrefix(uri, "spec.openapis.org/oas/3.1/dialect"):
		return "oas-3.1"
	default:
		return "unknown"
	}
}

// RootType bepaalt het root type van een JSON Schema. Schemas zonder expliciet
// type (bijv. alleen allOf of $ref) krijgen "unknown".
func RootType(content map[string]any) string {
	switch v := content["type"].(type) {
	case string:
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return "unknown"
}

// BuildSchema construeert een models.Schema op basis van het geparste JSON
// Schema en de request body.
func BuildSchema(res *SchemaResult, requestBody models.SchemaPost, label string) *models.Schema {
	schema := &models.Schema{
		Id: shortid.MustGenerate(),
	}
	if strings.TrimSpace(requestBody.Id) != "" {
		schema.Id = strings.TrimSpace(requestBody.Id)
	}
	schema.CreatedAt = time.Now()

	populateSchemaFromContent(schema, res, requestBody, label)

	return schema
}

// UpdateSchemaFromContent muteert een bestaand models.Schema met waarden uit
// het (opnieuw) opgehaalde JSON Schema.
func UpdateSchemaFromContent(schema *models.Schema, res *SchemaResult, requestBody models.SchemaPost, label string) {
	populateSchemaFromContent(schema, res, requestBody, label)
}

func populateSchemaFromContent(schema *models.Schema, res *SchemaResult, requestBody models.SchemaPost, label string) {
	if schema == nil || res == nil {
		return
	}

	schema.Title = stringValue(res.Content, "title")
	schema.Description = stringValue(res.Content, "description")
	schema.Dialect = res.Dialect
	schema.RootType = res.RootType
	schema.Content = res.Content
	schema.SchemaUrl = requestBody.SchemaUrl
	schema.LastCrawledAt = time.Now()

	if strings.TrimSpace(requestBody.OrganisationUri) != "" {
		schema.Organisation = &models.Organisation{
			Uri:   requestBody.OrganisationUri,
			Label: label,
		}
		schema.OrganisationID = &requestBody.OrganisationUri
	}

	schema.ContactName = requestBody.Contact.Name
	schema.ContactEmail = requestBody.Contact.Email
	schema.ContactUrl = requestBody.Contact.URL
}

// ValidateSchema verzamelt validatiefouten over een opgebouwd schema, naar
// het patroon van openapi.ValidateApi in don-api-register.
func ValidateSchema(schema *models.Schema) []problem.ErrorDetail {
	var invalids []problem.ErrorDetail

	if strings.TrimSpace(schema.ContactName) == "" {
		invalids = append(invalids, problem.ErrorDetail{
			In:       "body",
			Location: "#/contact/name",
			Code:     "required",
			Detail:   "contact.name is verplicht",
		})
	}
	if strings.TrimSpace(schema.ContactEmail) == "" {
		invalids = append(invalids, problem.ErrorDetail{
			In:       "body",
			Location: "#/contact/email",
			Code:     "required",
			Detail:   "contact.email is verplicht",
		})
	}
	if strings.TrimSpace(schema.ContactUrl) == "" {
		invalids = append(invalids, problem.ErrorDetail{
			In:       "body",
			Location: "#/contact/url",
			Code:     "required",
			Detail:   "contact.url is verplicht",
		})
	}
	if len(schema.Content) == 0 {
		invalids = append(invalids, problem.ErrorDetail{
			In:       "body",
			Location: "#/schemaUrl",
			Code:     "invalid",
			Detail:   "schema-inhoud ontbreekt",
		})
	}

	return invalids
}

func fetchRawSchema(ctx context.Context, input SchemaInput, opts FetchOpts) ([]byte, string, error) {
	if body := strings.TrimSpace(input.SchemaBody); body != "" {
		return []byte(body), "", nil
	}
	schemaURL := strings.TrimSpace(input.SchemaUrl)
	if schemaURL == "" {
		return nil, "", fmt.Errorf("geen schemaUrl opgegeven")
	}
	cli := opts.HTTPClient
	if cli == nil {
		cli = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, schemaURL, nil)
	if err != nil {
		return nil, "", err
	}
	if opts.Origin != "" {
		req.Header.Set("Origin", opts.Origin)
	}
	req.Header.Set("Accept", "application/schema+json, application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("kan schema niet ophalen: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxSchemaBytes))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return nil, "", fmt.Errorf("kan schema niet lezen: %w", readErr)
	}
	if closeErr != nil {
		return nil, "", fmt.Errorf("kan schema response body niet sluiten: %w", closeErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", &HTTPStatusError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}
	return body, resp.Header.Get("Content-Type"), nil
}

func stringValue(content map[string]any, key string) string {
	if v, ok := content[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}
