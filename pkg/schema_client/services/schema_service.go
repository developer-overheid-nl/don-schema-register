package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	commontypesense "github.com/developer-overheid-nl/don-register-common/typesense"
	httpclient "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/httpclient"
	jsonschema "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/jsonschema"
	problem "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/problem"
	schemaanalysis "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/schemaanalysis"
	util "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/util"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/schemas"
	"github.com/teris-io/shortid"
)

// fetchOrigin wordt als Origin-header meegestuurd bij het ophalen van schemas.
const fetchOrigin = "https://developer.overheid.nl"

// defaultStaticBaseURL is de root van de originele schemabestanden in don-static.
const defaultStaticBaseURL = "https://static.developer.overheid.nl/schemas/"

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

	s.refreshAnalysisAndPublish(ctx, nil, *schema)
	if fresh, err := s.repo.GetSchemaByID(ctx, schema.Id); err == nil && fresh != nil {
		schema = fresh
	}

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
	if err := s.saveSchemaUpdate(ctx, schema, request, res); err != nil {
		return nil, err
	}

	s.refreshAnalysisAndPublish(ctx, nil, *schema)
	if fresh, err := s.repo.GetSchemaByID(ctx, schema.Id); err == nil && fresh != nil {
		schema = fresh
	}

	updated := util.ToSchemaSummary(schema)
	return &updated, nil
}

// saveSchemaUpdate valideert en bewaart een bijgewerkt schema zonder de analyse
// te draaien; de aanroeper is verantwoordelijk voor de analyse.
func (s *SchemaService) saveSchemaUpdate(ctx context.Context, schema *models.Schema, request models.SchemaPost, res *jsonschema.SchemaResult) error {
	if schema == nil {
		return fmt.Errorf("schema ontbreekt")
	}
	if res == nil {
		return fmt.Errorf("schema resultaat ontbreekt")
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
		return problem.NewBadRequest(
			"Validatie mislukt: ontbrekende of ongeldige eigenschappen",
			invalids...,
		)
	}

	return s.repo.UpdateSchema(ctx, schema)
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
	var changedSchemas []models.Schema
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
		// Analyse draait eenmalig aan het einde van de refresh.
		if err := s.saveSchemaUpdate(ctx, &schema, request, res); err != nil {
			log.Printf("[refresh] schema=%s bijwerken mislukt: %v", schema.Id, err)
			continue
		}
		changedSchemas = append(changedSchemas, schema)
		updated++
	}
	if updated > 0 {
		// Eén analyse voor alle bijgewerkte schemas; publiceert ze daarna met
		// de nieuwste analysevelden.
		s.refreshAnalysisAndPublish(ctx, nil, changedSchemas...)
	}
	return updated, nil
}

// HarvestStaticSchemas slaat de uit don-static opgehaalde schemabestanden op en
// verwijdert (alleen bij een complete harvest) eerder geharveste schemas waarvan
// het bestand niet meer in de listing voorkomt.
func (s *SchemaService) HarvestStaticSchemas(ctx context.Context, harvest models.StaticHarvest) (stored int, err error) {
	baseURL := strings.TrimSpace(harvest.BaseURL)
	if baseURL == "" {
		baseURL = defaultStaticBaseURL
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	now := time.Now()
	storedSchemas := make([]*models.Schema, 0, len(harvest.Files))
	raws := make(map[string][]byte, len(harvest.Files))

	// SaveSchema schrijft de rij zonder analysevelden; draai de analyse daarom
	// op elk uitstappad nadat minstens één schema is opgeslagen (behalve bij
	// een geannuleerde context).
	defer func() {
		if len(storedSchemas) == 0 || ctx.Err() != nil {
			return
		}
		primary := make([]models.Schema, 0, len(storedSchemas))
		for _, schema := range storedSchemas {
			primary = append(primary, *schema)
		}
		s.refreshAnalysisAndPublish(ctx, raws, primary...)
	}()
	for _, file := range harvest.Files {
		if err = ctx.Err(); err != nil {
			return stored, err
		}
		fileURL := strings.TrimSpace(file.URL)
		if fileURL == "" || len(file.Raw) == 0 {
			continue
		}
		res, err := jsonschema.ParseValidateAndHash(file.Raw, "application/schema+json")
		if err != nil {
			log.Printf("[static] schema %q parsen mislukt: %v", fileURL, err)
			continue
		}

		relative := staticRelativePath(baseURL, fileURL)
		name := path.Base(relative)
		sourceMetaPath := relative
		root := ""
		if idx := strings.Index(relative, "/"); idx >= 0 {
			root = relative[:idx]
		}
		schema := &models.Schema{
			Id:                   shortid.MustGenerate(),
			SchemaUrl:            fileURL,
			Title:                contentStringWithFallback(res.Content, "title", name),
			Description:          contentStringWithFallback(res.Content, "description"),
			Dialect:              res.Dialect,
			RootType:             res.RootType,
			Content:              res.Content,
			Hash:                 res.Hash,
			SourceMetaName:       name,
			SourceMetaPath:       sourceMetaPath,
			SourceMetaRoot:       root,
			SourceMetaIdentifier: fileURL,
			LastCrawledAt:        now,
		}
		if err = s.repo.SaveSchema(ctx, schema); err != nil {
			return stored, err
		}
		storedSchemas = append(storedSchemas, schema)
		raws[schema.Id] = file.Raw
		stored++
	}

	if harvest.Complete && len(harvest.ListedURLs) == 0 {
		log.Printf("[static] waarschuwing: complete harvest zonder bestanden; niets verwijderd")
	} else if harvest.Complete {
		if err = s.deleteVanishedStaticSchemas(ctx, harvest.ListedURLs); err != nil {
			return stored, err
		}
	}

	return stored, nil
}

// deleteVanishedStaticSchemas verwijdert geharveste schemas (niet-lege
// SourceMetaIdentifier) die niet meer in de static-listing staan.
func (s *SchemaService) deleteVanishedStaticSchemas(ctx context.Context, listedURLs []string) error {
	listed := make(map[string]struct{}, len(listedURLs))
	for _, u := range listedURLs {
		listed[strings.TrimSpace(u)] = struct{}{}
	}
	all, err := s.repo.AllSchemas(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range all {
		identifier := strings.TrimSpace(candidate.SourceMetaIdentifier)
		if identifier == "" {
			continue
		}
		if _, ok := listed[identifier]; ok {
			continue
		}
		if err := s.repo.DeleteSchema(ctx, candidate.Id); err != nil {
			return err
		}
		log.Printf("[static] schema %s (%s) verwijderd: bestand niet meer aanwezig", candidate.Id, identifier)
		if err := deleteSchemaFromTypesense(ctx, candidate.Id); err != nil {
			log.Printf("[static] schema %s uit Typesense verwijderen mislukt: %v", candidate.Id, err)
		}
	}
	return nil
}

// staticRelativePath geeft het pad van een bestands-URL relatief aan de base-URL,
// zonder .schema.json/.json-extensie en URL-decoded (bv. "don/Api").
func staticRelativePath(baseURL, fileURL string) string {
	rel := strings.TrimPrefix(fileURL, baseURL)
	if rel == fileURL {
		if u, err := url.Parse(fileURL); err == nil {
			rel = strings.TrimPrefix(u.Path, "/")
		}
	}
	if unescaped, err := url.PathUnescape(rel); err == nil {
		rel = unescaped
	}
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimSuffix(rel, ".json")
	return strings.TrimSuffix(rel, ".schema")
}

// RefreshAnalysis berekent voor alle schemas in het register de dependencies,
// dependents, health, groottes en dialect met schemaanalysis en slaat alleen
// de schemas op (en publiceert ze naar Typesense) waarvan de analyse veranderde.
func (s *SchemaService) RefreshAnalysis(ctx context.Context) error {
	changed, err := s.runAnalysis(ctx, nil)
	for _, schema := range changed {
		go s.publishToTypesense(schema)
	}
	return err
}

// refreshAnalysisAndPublish draait de analyse (fouten worden gelogd, nooit
// teruggegeven) en publiceert de opgegeven schemas plus alle schemas waarvan de
// analyse veranderde, elk precies een keer en met de nieuwste analysevelden.
func (s *SchemaService) refreshAnalysisAndPublish(ctx context.Context, raws map[string][]byte, primary ...models.Schema) {
	changed, err := s.runAnalysis(ctx, raws)
	if err != nil {
		log.Printf("[analyse] analyse mislukt: %v", err)
	}
	latest := make(map[string]models.Schema, len(changed))
	for _, schema := range changed {
		latest[schema.Id] = schema
	}
	published := make(map[string]bool, len(primary)+len(changed))
	for _, schema := range primary {
		if newer, ok := latest[schema.Id]; ok {
			schema = newer
		}
		published[schema.Id] = true
		go s.publishToTypesense(schema)
	}
	for _, schema := range changed {
		if !published[schema.Id] {
			go s.publishToTypesense(schema)
		}
	}
}

// runAnalysis voert de analyse uit en geeft de schemas terug die zijn bijgewerkt.
// raws bevat optioneel de originele bestandsinhoud per schema-id; zonder raw
// wordt de grootte uit de content afgeleid (of, voor geharveste schemas, de
// eerder vastgelegde bestandsgrootte behouden).
func (s *SchemaService) runAnalysis(ctx context.Context, raws map[string][]byte) ([]models.Schema, error) {
	all, byKey, lookup, err := s.analysisIndex(ctx)
	if err != nil {
		return nil, err
	}
	bundledURLTemplate := bundleURLTemplate()

	results := make([]schemaanalysis.Result, len(all))
	type incomingEdge struct {
		dep    models.SourceMetaDependency
		source models.Schema
	}
	incoming := make(map[string][]incomingEdge)
	for i, schema := range all {
		results[i] = schemaanalysis.Analyze(schema.Content, raws[schema.Id], schema.SchemaUrl, lookup)
		for _, dep := range results[i].Dependencies {
			target, ok := byKey[strings.TrimSpace(dep.To)]
			if ok && target.Id != schema.Id {
				incoming[target.Id] = append(incoming[target.Id], incomingEdge{dep: dep, source: schema})
			}
		}
	}

	var changed []models.Schema
	for i := range all {
		schema := all[i]
		res := results[i]

		outgoing := append([]models.SourceMetaDependency(nil), res.Dependencies...)
		sort.SliceStable(outgoing, func(a, b int) bool { return outgoing[a].At < outgoing[b].At })
		in := incoming[schema.Id]
		sort.SliceStable(in, func(a, b int) bool {
			if in[a].dep.From != in[b].dep.From {
				return in[a].dep.From < in[b].dep.From
			}
			if in[a].source.Id != in[b].source.Id {
				return in[a].source.Id < in[b].source.Id
			}
			return in[a].dep.At < in[b].dep.At
		})
		details := make([]models.SourceMetaDependency, 0, len(outgoing)+len(in))
		for _, dep := range outgoing {
			details = append(details, enrichDependencyEdge(dep, schema, byKey))
		}
		for _, edge := range in {
			details = append(details, enrichDependencyEdge(edge.dep, edge.source, byKey))
		}

		bytes := res.Bytes
		if len(raws[schema.Id]) == 0 && strings.TrimSpace(schema.SourceMetaIdentifier) != "" && schema.SourceMetaBytes > 0 {
			bytes = schema.SourceMetaBytes
		}

		next := schema
		next.SourceMetaDependencyDetails = details
		next.SourceMetaDependencies = res.DependencyCount
		next.SourceMetaHealth = res.Health
		next.SourceMetaHealthIssues = res.HealthIssues
		next.SourceMetaBytes = bytes
		next.SourceMetaBytesBundled = res.BytesBundled
		next.SourceMetaBaseDialect = res.BaseDialect
		next.SourceMetaDialect = res.Dialect
		next.SourceMetaBundledUrl = strings.ReplaceAll(bundledURLTemplate, "{id}", url.PathEscape(schema.Id))

		if !analysisChanged(schema, next) {
			continue
		}
		if err := s.repo.UpdateSchemaAnalysis(ctx, &next); err != nil {
			return changed, err
		}
		changed = append(changed, next)
	}
	return changed, nil
}

// analysisIndex haalt alle schemas op (gesorteerd op id) en bouwt de lookup op
// identifier, schemaUrl en absolute $id (zonder fragment). Sleutelbotsingen
// worden deterministisch opgelost: het schema met de laagste id wint.
func (s *SchemaService) analysisIndex(ctx context.Context) ([]models.Schema, map[string]models.Schema, schemaanalysis.Lookup, error) {
	all, err := s.repo.AllSchemas(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	sort.SliceStable(all, func(a, b int) bool { return all[a].Id < all[b].Id })
	byKey := make(map[string]models.Schema, len(all)*3)
	for _, schema := range all {
		for _, key := range analysisKeys(schema) {
			if _, exists := byKey[key]; !exists {
				byKey[key] = schema
			}
		}
	}
	lookup := func(uri string) (map[string]any, bool) {
		schema, ok := byKey[strings.TrimSpace(uri)]
		if !ok || schema.Content == nil {
			return nil, false
		}
		return schema.Content, true
	}
	return all, byKey, lookup, nil
}

// analysisLookup geeft dezelfde register-lookup als de analyse gebruikt.
func (s *SchemaService) analysisLookup(ctx context.Context) (schemaanalysis.Lookup, error) {
	_, _, lookup, err := s.analysisIndex(ctx)
	return lookup, err
}

// BundleSchema geeft het gebundelde schema (alle transitieve refs binnen het
// register ingevoegd onder $defs). Geeft nil, nil bij een onbekend id.
func (s *SchemaService) BundleSchema(ctx context.Context, id string) (map[string]any, error) {
	schema, err := s.RetrieveSchemaModel(ctx, id)
	if err != nil {
		return nil, err
	}
	if schema == nil || len(schema.Content) == 0 {
		return nil, nil
	}
	lookup, err := s.analysisLookup(ctx)
	if err != nil {
		return nil, err
	}
	base := schemaanalysis.BaseURI(schema.Content, schema.SchemaUrl)
	bundle, _ := schemaanalysis.Bundle(schema.Content, base, lookup)
	return bundle, nil
}

// bundleURLTemplate geeft het publieke URL-sjabloon voor gebundelde schemas;
// {id} wordt per schema vervangen.
func bundleURLTemplate() string {
	if t := strings.TrimSpace(os.Getenv("SCHEMA_BUNDLE_URL_TEMPLATE")); t != "" {
		return t
	}
	return "https://schemas.developer.overheid.nl/schemas/download/{id}/bundle.json"
}

// enrichDependencyEdge zet From* op het werkelijke bronschema en To* via de
// register-lookup.
func enrichDependencyEdge(dep models.SourceMetaDependency, source models.Schema, byKey map[string]models.Schema) models.SourceMetaDependency {
	dep, _ = withSourceMetaDependencySchema(dep, "from", source, true)
	target, found := byKey[strings.TrimSpace(dep.To)]
	dep, _ = withSourceMetaDependencySchema(dep, "to", target, found)
	return dep
}

// analysisKeys geeft de sleutels waarop een schema binnen het register
// gevonden kan worden: identifier, schemaUrl en absolute $id zonder fragment.
func analysisKeys(schema models.Schema) []string {
	var keys []string
	add := func(k string) {
		if k = strings.TrimSpace(k); k != "" {
			keys = append(keys, k)
		}
	}
	add(schema.SourceMetaIdentifier)
	add(schema.SchemaUrl)
	if id, ok := schema.Content["$id"].(string); ok {
		if u, err := url.Parse(strings.TrimSpace(id)); err == nil && u.IsAbs() {
			u.Fragment = ""
			add(u.String())
		}
	}
	return keys
}

// analysisChanged vergelijkt de analysevelden van twee schemas; lege en nil
// slices gelden als gelijk.
func analysisChanged(a, b models.Schema) bool {
	return a.SourceMetaDependencies != b.SourceMetaDependencies ||
		a.SourceMetaHealth != b.SourceMetaHealth ||
		a.SourceMetaBytes != b.SourceMetaBytes ||
		a.SourceMetaBytesBundled != b.SourceMetaBytesBundled ||
		a.SourceMetaBaseDialect != b.SourceMetaBaseDialect ||
		a.SourceMetaDialect != b.SourceMetaDialect ||
		a.SourceMetaBundledUrl != b.SourceMetaBundledUrl ||
		(len(a.SourceMetaDependencyDetails) != 0 || len(b.SourceMetaDependencyDetails) != 0) &&
			!reflect.DeepEqual(a.SourceMetaDependencyDetails, b.SourceMetaDependencyDetails) ||
		(len(a.SourceMetaHealthIssues) != 0 || len(b.SourceMetaHealthIssues) != 0) &&
			!reflect.DeepEqual(a.SourceMetaHealthIssues, b.SourceMetaHealthIssues)
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
		schemaURL = sourceMetaSchemaFrontendPath(schema.Id)
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

func sourceMetaSchemaFrontendPath(id string) string {
	return "schemas/" + url.PathEscape(id)
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
		buildRootTypeGroup(p, counts),
		buildDialectGroup(p, counts),
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
