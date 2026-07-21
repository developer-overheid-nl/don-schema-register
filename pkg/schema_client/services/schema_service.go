package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	commontypesense "github.com/developer-overheid-nl/don-register-common/typesense"
	httpclient "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/httpclient"
	jsonschema "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/jsonschema"
	problem "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/problem"
	util "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/util"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/schemas"
	"github.com/teris-io/shortid"
)

// fetchOrigin wordt als Origin-header meegestuurd bij het ophalen van schemas.
const fetchOrigin = "https://developer.overheid.nl"

const schemaRegisterArtifactBaseURL = "https://api.don.projects.digilab.network/schema-register"

// ErrNeedsPost geeft aan dat een PUT verwijst naar een schema dat (nog) niet
// bestaat en via POST geregistreerd moet worden.
var ErrNeedsPost = errors.New("schema must be registered via POST")

// SchemaService implementeert de business-logica van het schema register.
type SchemaService struct {
	repo schemas.SchemasRepository
}

// NewSchemaService Constructor-functie
func NewSchemaService(repo schemas.SchemasRepository) *SchemaService {
	return &SchemaService{
		repo: repo,
	}
}

func (s *SchemaService) ListSchemas(ctx context.Context, p *models.ListSchemasParams) ([]models.SchemaSummary, models.Pagination, error) {
	if p == nil {
		p = &models.ListSchemasParams{}
	}

	allSchemas, pagination, err := s.repo.GetSchemas(ctx, p.Page, p.PerPage, p.SchemaFilters())
	if err != nil {
		return nil, models.Pagination{}, err
	}

	dtos := make([]models.SchemaSummary, len(allSchemas))
	for i, schema := range allSchemas {
		dtos[i] = util.ToSchemaSummary(&schema)
	}

	return dtos, pagination, nil
}

func (s *SchemaService) RetrieveSchema(ctx context.Context, id string) (*models.SchemaDetail, error) {
	if err := validateSchemaID(id); err != nil {
		return nil, err
	}
	schema, err := s.repo.GetSchemaByID(ctx, id)
	if err != nil || schema == nil {
		return nil, err
	}
	detail := util.ToSchemaDetail(schema)
	return detail, nil
}

// RetrieveSchemaModel haalt het volledige opgeslagen schema-record op,
// inclusief content (voor het raw-content endpoint).
func (s *SchemaService) RetrieveSchemaModel(ctx context.Context, id string) (*models.Schema, error) {
	if err := validateSchemaID(id); err != nil {
		return nil, err
	}
	return s.repo.GetSchemaByID(ctx, id)
}

func (s *SchemaService) SearchSchemas(ctx context.Context, p *models.ListSchemasSearchParams) ([]models.SchemaSummary, models.Pagination, error) {
	if p == nil {
		p = &models.ListSchemasSearchParams{}
	}
	trimmed := strings.TrimSpace(p.Query)
	if trimmed == "" {
		return nil, models.Pagination{}, problem.NewBadRequest("Invalid input",
			queryError("q", "required", "q is required"),
		)
	}
	matched, pagination, err := s.repo.SearchSchemas(ctx, p.Page, p.PerPage, p.Organisation, trimmed)
	if err != nil {
		return nil, models.Pagination{}, err
	}
	results := make([]models.SchemaSummary, len(matched))
	for i := range matched {
		results[i] = util.ToSchemaSummary(&matched[i])
	}
	return results, pagination, nil
}

// CreateSchemaFromInput registreert een nieuw JSON Schema, naar het patroon
// van CreateApiFromOas in don-api-register: ophalen, valideren, hashen,
// organisatie resolven en opslaan.
func (s *SchemaService) CreateSchemaFromInput(requestBody models.SchemaPost) (*models.SchemaSummary, error) {
	ctx := context.Background()

	// 1) Strict validate + hash
	res, err := jsonschema.FetchParseValidateAndHash(ctx, jsonschema.SchemaInput{
		SchemaUrl:  requestBody.SchemaUrl,
		SchemaBody: requestBody.SchemaBody,
	}, jsonschema.FetchOpts{Origin: fetchOrigin})
	if err != nil {
		return nil, problem.NewBadRequest(err.Error())
	}

	// 2) Organisatie resolven; onbekende organisaties worden via TOOI
	// opgehaald en opgeslagen.
	label, shouldSaveOrg, err := s.resolveOrganisationLabel(ctx, requestBody.OrganisationUri)
	if err != nil {
		return nil, err
	}

	// 3) Build & validate
	schema := jsonschema.BuildSchema(res, requestBody, label)
	schema.Hash = res.Hash
	if shouldSaveOrg && schema.Organisation != nil {
		if err := s.repo.SaveOrganisatie(schema.Organisation); err != nil {
			return nil, problem.NewInternalServerError("kan organisatie niet opslaan: " + err.Error())
		}
	}
	if invalids := jsonschema.ValidateSchema(schema); len(invalids) > 0 {
		return nil, problem.NewBadRequest(
			"Validatie mislukt: ontbrekende of ongeldige eigenschappen",
			invalids...,
		)
	}

	// 4) Sla op in DB
	if err := s.repo.SaveSchema(ctx, schema); err != nil {
		return nil, problem.NewInternalServerError("kan schema niet opslaan: " + err.Error())
	}

	schemaCopy := *schema
	go s.publishToTypesense(schemaCopy)

	created := util.ToSchemaSummary(schema)
	return &created, nil
}

// UpdateSchemaFromInput werkt een bestaand schema bij, naar het patroon van
// UpdateOasUri in don-api-register: eigenaar-check, opnieuw ophalen en
// dezelfde stappen als een POST uitvoeren.
func (s *SchemaService) UpdateSchemaFromInput(ctx context.Context, body *models.UpdateSchemaInput) (*models.SchemaSummary, error) {
	if err := validateSchemaID(body.Id); err != nil {
		return nil, err
	}
	schema, err := s.repo.GetSchemaByID(ctx, body.Id)
	if err != nil {
		return nil, fmt.Errorf("databasefout: %w", err)
	}
	if schema == nil {
		return nil, fmt.Errorf("%w: %s", ErrNeedsPost, body.SchemaUrl)
	}
	if ownerURI := deriveOrganisationURI(schema); ownerURI == "" || ownerURI != strings.TrimSpace(body.OrganisationUri) {
		return nil, problem.NewForbidden("organisationUri komt niet overeen met eigenaar van dit schema")
	}

	res, err := jsonschema.FetchParseValidateAndHash(ctx, jsonschema.SchemaInput{
		SchemaUrl:  body.SchemaUrl,
		SchemaBody: body.SchemaBody,
	}, jsonschema.FetchOpts{Origin: fetchOrigin})
	if err != nil {
		return nil, problem.NewBadRequest(err.Error())
	}

	return s.applySchemaUpdate(ctx, schema, models.SchemaPost{
		Id:              body.Id,
		SchemaUrl:       body.SchemaUrl,
		SchemaBody:      body.SchemaBody,
		OrganisationUri: body.OrganisationUri,
		Contact:         body.Contact,
	}, res)
}

func (s *SchemaService) applySchemaUpdate(ctx context.Context, schema *models.Schema, request models.SchemaPost, res *jsonschema.SchemaResult) (*models.SchemaSummary, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema ontbreekt")
	}
	if res == nil {
		return nil, fmt.Errorf("schema resultaat ontbreekt")
	}

	orgLabel := ""
	if schema.Organisation != nil {
		orgLabel = schema.Organisation.Label
	} else if trimmed := strings.TrimSpace(request.OrganisationUri); trimmed != "" {
		if org, err := s.repo.FindOrganisationByURI(ctx, trimmed); err == nil && org != nil {
			orgLabel = org.Label
		}
	}

	jsonschema.UpdateSchemaFromContent(schema, res, request, orgLabel)
	schema.Hash = res.Hash
	if schema.Organisation != nil && schema.OrganisationID == nil {
		schema.OrganisationID = &schema.Organisation.Uri
	}
	if invalids := jsonschema.ValidateSchema(schema); len(invalids) > 0 {
		return nil, problem.NewBadRequest(
			"Validatie mislukt: ontbrekende of ongeldige eigenschappen",
			invalids...,
		)
	}

	if err := s.repo.UpdateSchema(ctx, schema); err != nil {
		return nil, err
	}

	schemaCopy := *schema
	go s.publishToTypesense(schemaCopy)

	updated := util.ToSchemaSummary(schema)
	return &updated, nil
}

// RefreshChangedSchemas haalt alle geregistreerde schemas met een schemaUrl
// opnieuw op, vergelijkt de hash en werkt het schema bij wanneer de remote
// inhoud gewijzigd is. Spiegel van RefreshChangedApis in don-api-register.
func (s *SchemaService) RefreshChangedSchemas(ctx context.Context) (int, error) {
	allSchemas, err := s.repo.AllSchemas(ctx)
	if err != nil {
		return 0, err
	}

	var updated int
	for _, candidate := range allSchemas {
		if err := ctx.Err(); err != nil {
			return updated, err
		}
		schemaURL := strings.TrimSpace(candidate.SchemaUrl)
		if schemaURL == "" {
			continue
		}
		if strings.TrimSpace(candidate.SourceMetaIdentifier) != "" {
			continue
		}

		res, err := jsonschema.FetchParseValidateAndHash(ctx, jsonschema.SchemaInput{
			SchemaUrl: schemaURL,
		}, jsonschema.FetchOpts{Origin: fetchOrigin})
		if err != nil {
			log.Printf("[refresh] schema=%s url=%s ophalen mislukt: %v", candidate.Id, schemaURL, err)
			continue
		}
		if res.Hash == candidate.Hash {
			continue
		}

		schema := candidate
		request := models.SchemaPost{
			Id:              schema.Id,
			SchemaUrl:       schemaURL,
			OrganisationUri: deriveOrganisationURI(&schema),
			Contact: models.Contact{
				Name:  schema.ContactName,
				URL:   schema.ContactUrl,
				Email: schema.ContactEmail,
			},
		}
		if _, err := s.applySchemaUpdate(ctx, &schema, request, res); err != nil {
			log.Printf("[refresh] schema=%s bijwerken mislukt: %v", schema.Id, err)
			continue
		}
		updated++
	}
	return updated, nil
}

func (s *SchemaService) HarvestSourceMetaSchemas(ctx context.Context, entries []models.SourceMetaSchemaMetadata) (int, error) {
	var stored int
	now := time.Now()
	storedSchemas := make([]*models.Schema, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return stored, err
		}

		identifier := strings.TrimSpace(entry.Identifier)
		if identifier == "" {
			continue
		}
		if len(entry.RawContent) == 0 {
			log.Printf("[sourcemeta] schema %q overgeslagen: inhoud ontbreekt", identifier)
			continue
		}
		res, err := jsonschema.ParseValidateAndHash(entry.RawContent, "application/schema+json")
		if err != nil {
			log.Printf("[sourcemeta] schema %q parsen mislukt: %v", identifier, err)
			continue
		}

		id := sourceMetaSchemaID(identifier)
		schema := &models.Schema{
			Id:                          id,
			SchemaUrl:                   sourceMetaSchemaArtifactURL(id),
			Title:                       contentStringWithFallback(res.Content, "title", entry.Name, identifier),
			Description:                 contentStringWithFallback(res.Content, "description", entry.Description),
			Dialect:                     res.Dialect,
			RootType:                    res.RootType,
			Content:                     res.Content,
			Hash:                        res.Hash,
			SourceMetaName:              strings.TrimSpace(entry.Name),
			SourceMetaIdentifier:        identifier,
			SourceMetaBytes:             entry.Bytes,
			SourceMetaBytesBundled:      entry.BytesBundled,
			SourceMetaBaseDialect:       strings.TrimSpace(entry.BaseDialect),
			SourceMetaDialect:           strings.TrimSpace(entry.Dialect),
			SourceMetaHealth:            entry.Health,
			SourceMetaDependencies:      entry.Dependencies,
			SourceMetaDependencyDetails: append([]models.SourceMetaDependency(nil), entry.DependencyDetails...),
			LastCrawledAt:               now,
		}
		if schema.Dialect == "unknown" && strings.TrimSpace(entry.BaseDialect) != "" {
			schema.Dialect = jsonschema.NormalizeDialect(entry.BaseDialect)
		}

		if err := s.repo.SaveSchema(ctx, schema); err != nil {
			return stored, err
		}
		if expectedSchemaURL := sourceMetaSchemaArtifactURL(schema.Id); schema.SchemaUrl != expectedSchemaURL {
			schema.SchemaUrl = expectedSchemaURL
			if err := s.repo.SaveSchema(ctx, schema); err != nil {
				return stored, err
			}
		}

		storedSchemas = append(storedSchemas, schema)
		stored++
	}

	if err := s.enrichSourceMetaDependencyDetails(ctx, storedSchemas); err != nil {
		return stored, err
	}
	for _, schema := range storedSchemas {
		schemaCopy := *schema
		go s.publishToTypesense(schemaCopy)
	}

	return stored, nil
}

func (s *SchemaService) enrichSourceMetaDependencyDetails(ctx context.Context, schemas []*models.Schema) error {
	if len(schemas) == 0 {
		return nil
	}

	allSchemas, err := s.repo.AllSchemas(ctx)
	if err != nil {
		return err
	}
	bySourceMetaIdentifier := make(map[string]models.Schema, len(allSchemas))
	for _, schema := range allSchemas {
		if identifier := strings.TrimSpace(schema.SourceMetaIdentifier); identifier != "" {
			bySourceMetaIdentifier[identifier] = schema
		}
	}

	for _, schema := range schemas {
		if len(schema.SourceMetaDependencyDetails) == 0 {
			continue
		}
		enriched, changed := enrichSourceMetaDependencies(schema.SourceMetaDependencyDetails, bySourceMetaIdentifier)
		if !changed {
			continue
		}
		schema.SourceMetaDependencyDetails = enriched
		if err := s.repo.SaveSchema(ctx, schema); err != nil {
			return err
		}
	}

	return nil
}

func enrichSourceMetaDependencies(
	dependencies []models.SourceMetaDependency,
	bySourceMetaIdentifier map[string]models.Schema,
) ([]models.SourceMetaDependency, bool) {
	enriched := make([]models.SourceMetaDependency, len(dependencies))
	changed := false
	for i, dependency := range dependencies {
		next := dependency

		fromSchema, hasFromSchema := bySourceMetaIdentifier[strings.TrimSpace(dependency.From)]
		var dependencyChanged bool
		next, dependencyChanged = withSourceMetaDependencySchema(next, "from", fromSchema, hasFromSchema)
		changed = dependencyChanged || changed

		toSchema, hasToSchema := bySourceMetaIdentifier[strings.TrimSpace(dependency.To)]
		next, dependencyChanged = withSourceMetaDependencySchema(next, "to", toSchema, hasToSchema)
		changed = dependencyChanged || changed

		enriched[i] = next
	}
	return enriched, changed
}

func withSourceMetaDependencySchema(
	dependency models.SourceMetaDependency,
	direction string,
	schema models.Schema,
	found bool,
) (models.SourceMetaDependency, bool) {
	id, schemaURL, title := "", "", ""
	if found {
		id = schema.Id
		schemaURL = schema.SchemaUrl
		title = schema.Title
	}

	switch direction {
	case "from":
		changed := dependency.FromSchemaId != id ||
			dependency.FromSchemaUrl != schemaURL ||
			dependency.FromSchemaTitle != title
		dependency.FromSchemaId = id
		dependency.FromSchemaUrl = schemaURL
		dependency.FromSchemaTitle = title
		return dependency, changed
	case "to":
		changed := dependency.ToSchemaId != id ||
			dependency.ToSchemaUrl != schemaURL ||
			dependency.ToSchemaTitle != title
		dependency.ToSchemaId = id
		dependency.ToSchemaUrl = schemaURL
		dependency.ToSchemaTitle = title
		return dependency, changed
	default:
		return dependency, false
	}
}

func sourceMetaSchemaID(identifier string) string {
	return shortid.MustGenerate()
}

func sourceMetaSchemaArtifactURL(id string) string {
	return schemaRegisterArtifactBaseURL + "/v1/schemas/" + url.PathEscape(id) + "/schema.json"
}

func contentStringWithFallback(content map[string]any, key string, fallbacks ...string) string {
	if value, ok := content[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	for _, fallback := range fallbacks {
		if trimmed := strings.TrimSpace(fallback); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (s *SchemaService) ListOrganisations(ctx context.Context, p *models.ListOrganisationsParams) ([]models.OrganisationSummary, models.Pagination, error) {
	organisations, pagination, err := s.repo.GetOrganisations(ctx, p.Page, p.PerPage)
	if err != nil {
		return nil, models.Pagination{}, err
	}

	orgSummaries := make([]models.OrganisationSummary, len(organisations))
	for i, org := range organisations {
		orgSummaries[i] = models.OrganisationSummary(org)
	}

	return orgSummaries, pagination, nil
}

// CreateOrganisation validates and stores a new organisation
func (s *SchemaService) CreateOrganisation(ctx context.Context, org *models.Organisation) (*models.Organisation, error) {
	org.Uri = strings.TrimSpace(org.Uri)
	org.Label = strings.TrimSpace(org.Label)

	if _, err := url.ParseRequestURI(org.Uri); err != nil {
		return nil, problem.NewBadRequest("Invalid input",
			bodyError("uri", "url", "must be a valid URL"),
		)
	}
	if lbl, err := httpclient.FetchOrganisationLabel(ctx, org.Uri); err == nil && strings.TrimSpace(lbl) != "" {
		org.Label = lbl
	}
	if org.Label == "" {
		return nil, problem.NewBadRequest("Invalid input",
			bodyError("label", "required", "label is required"),
		)
	}
	existing, err := s.repo.FindOrganisationByURI(ctx, org.Uri)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, problem.New(http.StatusConflict, "Organisation already exists",
			bodyError("uri", "conflict", "organisation already exists"),
		)
	}
	if err := s.repo.SaveOrganisatie(org); err != nil {
		return nil, err
	}
	return org, nil
}

// resolveOrganisationLabel zoekt de organisatie op in de database en valt
// terug op een TOOI label-fetch voor onbekende organisaties (zoals
// CreateApiFromOas in don-api-register doet).
func (s *SchemaService) resolveOrganisationLabel(ctx context.Context, organisationUri string) (label string, shouldSave bool, err error) {
	trimmed := strings.TrimSpace(organisationUri)
	if org, err := s.repo.FindOrganisationByURI(ctx, trimmed); err != nil {
		return "", false, problem.NewInternalServerError("kan organisatie niet ophalen: " + err.Error())
	} else if org != nil {
		return org.Label, false, nil
	}

	if _, err := url.ParseRequestURI(trimmed); err != nil {
		return "", false, problem.NewBadRequest("Invalid input",
			bodyError("organisationUri", "url", "must be a valid URL"),
		)
	}
	lbl, err := httpclient.FetchOrganisationLabel(ctx, trimmed)
	if err != nil {
		return "", false, problem.NewBadRequest(fmt.Sprintf("fout bij ophalen organisatie: %s", err))
	}
	return lbl, true, nil
}

func deriveOrganisationURI(schema *models.Schema) string {
	if schema == nil {
		return ""
	}
	if schema.Organisation != nil && strings.TrimSpace(schema.Organisation.Uri) != "" {
		return strings.TrimSpace(schema.Organisation.Uri)
	}
	if schema.OrganisationID != nil {
		return strings.TrimSpace(*schema.OrganisationID)
	}
	return ""
}

func (s *SchemaService) publishToTypesense(schema models.Schema) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := publishSchemaToTypesense(ctx, &schema); err != nil {
		if errors.Is(err, commontypesense.ErrDisabled) {
			return
		}
		log.Printf("[typesense] indexing failed for schema=%s: %v", schema.Id, err)
	}
}

// PublishAllSchemasToTypesense pushes every stored schema to Typesense.
func (s *SchemaService) PublishAllSchemasToTypesense(ctx context.Context) error {
	if !schemaTypesenseEnabled() {
		log.Printf("[typesense] indexing disabled; skip bulk publish")
		return nil
	}

	allSchemas, err := s.repo.AllSchemas(ctx)
	if err != nil {
		return err
	}

	for _, schema := range allSchemas {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		schemaCopy := schema
		itemCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := publishSchemaToTypesense(itemCtx, &schemaCopy)
		cancel()
		if err != nil {
			if errors.Is(err, commontypesense.ErrDisabled) {
				log.Printf("[typesense] indexing disabled tijdens bulk run; stop")
				return nil
			}
			log.Printf("[typesense] bulk indexing failed for schema=%s: %v", schemaCopy.Id, err)
		}
	}
	return nil
}

func (s *SchemaService) GetSchemaFilters(ctx context.Context, p *models.SchemaFiltersParams) ([]models.FilterGroup, error) {
	counts, err := s.repo.GetSchemaFilterCounts(ctx, p)
	if err != nil {
		return nil, err
	}
	if p == nil {
		p = &models.SchemaFiltersParams{}
	}
	groups := []models.FilterGroup{
		buildDialectGroup(p, counts),
		buildRootTypeGroup(p, counts),
	}
	for _, g := range groups {
		if err := g.Validate(); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func bodyError(field, code, detail string) problem.ErrorDetail {
	return problem.ErrorDetail{
		In:       "body",
		Location: fmt.Sprintf("#/%s", field),
		Code:     code,
		Detail:   detail,
	}
}

func pathError(field, code, detail string) problem.ErrorDetail {
	return problem.ErrorDetail{
		In:       "path",
		Location: fmt.Sprintf("#/%s", field),
		Code:     code,
		Detail:   detail,
	}
}

func queryError(field, code, detail string) problem.ErrorDetail {
	return problem.ErrorDetail{
		In:       "query",
		Location: fmt.Sprintf("#/%s", field),
		Code:     code,
		Detail:   detail,
	}
}

func validateSchemaID(id string) error {
	if id == "" {
		return problem.NewBadRequest("Invalid input",
			pathError("id", "required", "id is required"),
		)
	}
	if strings.ContainsRune(id, 0) {
		return problem.NewBadRequest("Invalid input",
			pathError("id", "invalid", "id must not contain NUL bytes"),
		)
	}
	if !utf8.ValidString(id) {
		return problem.NewBadRequest("Invalid input",
			pathError("id", "invalid", "id must be valid UTF-8"),
		)
	}
	return nil
}
