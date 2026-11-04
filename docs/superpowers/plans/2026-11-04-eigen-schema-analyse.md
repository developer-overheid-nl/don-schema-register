# Eigen schema-analyse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SourceMeta One vervangen door een eigen harvester voor `don-static` en eigen analyse (dependencies, health, bytes, bundel) in de Go-backend, met gelijkblijvend API-contract.

**Architecture:** Een nieuw pakket `pkg/schema_client/helpers/schemaanalysis` rekent alles uit op basis van de schema-inhoud plus een lookup van andere schema's in het register. Een nieuwe `StaticHarvester` loopt de nginx JSON directory-listing van static door en haalt de originele bestanden op. De service slaat geharveste schema's op, ruimt verdwenen schema's op en draait daarna de analyse voor alle schema's; een nieuw endpoint serveert de bundel.

**Tech Stack:** Go 1.26, Gin/Fizz/tonic, GORM (Postgres; tests met sqlite), testify.

**Spec:** `docs/superpowers/specs/2026-11-04-eigen-schema-analyse-design.md`

## Global Constraints

- Geen enkele afhankelijkheid van SourceMeta (server, CLI of library); `go.mod` krijgt geen nieuwe dependency.
- API-velden en DB-kolommen blijven gelijk (`sourceMeta*`-namen blijven); het response-formaat van `GET /v1/schemas/{id}` verandert niet.
- `$id` en `$ref` in opgeslagen content worden nooit herschreven.
- Refs worden alleen geresolved binnen het register; geen extra netwerkverkeer voor refs.
- Code-commentaar en logregels in het Nederlands, zoals de bestaande code.
- Eén changie-fragment in `.changes/unreleased/` voor deze branch.
- `api/openapi.json` moet ADR-score 100 houden.
- Commits als de gebruiker, zonder Claude-attributie.

## Review Focus

1. Een `$ref` met fragment (`Organisation.schema.json#/$defs/x`) moet als dependency naar het schema zonder fragment tellen, en de bundel moet het hele doelschema insluiten. → test in Task 1 en Task 2.
2. Cyclische refs (A → B → A) mogen de bundel en de analyse niet laten vastlopen. → test in Task 2.
3. Een directory-listing die halverwege faalt mag geen records laten verwijderen. → test in Task 5.
4. Een bestand in de listing dat geen geldige JSON is, houdt zijn bestaande record. → test in Task 5.
5. Een schema zonder `$id` dat via `schemaBody` (POST) is geregistreerd: relatieve refs kunnen niet resolven en moeten als onopgelost health-issue verschijnen, zonder panic. → test in Task 3.

---

### Task 1: Refs, bytes en dialect

**Files:**
- Create: `pkg/schema_client/helpers/schemaanalysis/refs.go`
- Test: `pkg/schema_client/helpers/schemaanalysis/refs_test.go`

**Interfaces:**
- Produces:
  - `type Ref struct { At string; Raw string; Target string }` — `At` is de JSON-pointer naar de `$ref` (bv. `/properties/lifecycle/$ref`), `Raw` de originele waarde, `Target` de absolute URI zonder fragment (leeg als niet te resolven).
  - `func ExternalRefs(content map[string]any, baseURI string) []Ref` — alle `$ref`s die niet met `#` beginnen, gesorteerd op `At`; resolved met `url.ResolveReference` tegen `baseURI`; bij lege of niet-absolute base en relatieve ref blijft `Target` leeg.
  - `func CompactBytes(raw []byte) int` — lengte van `json.Compact(raw)`; bij fout `len(raw)`.
  - `func BaseURI(content map[string]any, fallback string) string` — `$id` als die absoluut is, anders `fallback`.

- [ ] **Step 1: Write the failing tests**
  - `TestExternalRefsResolvesRelativeToBase`: content met `properties.lifecycle.$ref = "ApiLifecycle.schema.json"` en `properties.organisation.$ref = "Organisation.schema.json#/$defs/x"`, base `https://static.developer.overheid.nl/schemas/don/Api.schema.json` → twee Refs met `At` `/properties/lifecycle/$ref` en `/properties/organisation/$ref`, `Target` resp. `https://static.developer.overheid.nl/schemas/don/ApiLifecycle.schema.json` en `.../Organisation.schema.json`.
  - `TestExternalRefsSkipsLocalRefs`: `"$ref": "#/$defs/a"` levert niets.
  - `TestExternalRefsWithoutBaseLeavesTargetEmpty`: base `""`, ref `"Other.schema.json"` → één Ref met `Target == ""`; absolute ref `https://x.nl/a.json` → `Target == "https://x.nl/a.json"`.
  - `TestExternalRefsWalksArraysAndEscapesPointer`: ref in `allOf[1]` en onder property `a/b` → `At` is `/allOf/1/$ref` en `/properties/a~1b/$ref`.
  - `TestCompactBytes`: `"{ \"a\": 1 }"` → 7.
- [ ] **Step 2: Run** `go test ./pkg/schema_client/helpers/schemaanalysis/ -run 'TestExternalRefs|TestCompactBytes'` — Expected: FAIL (package/functions missing).
- [ ] **Step 3: Implement** the four functions in `refs.go`.
- [ ] **Step 4: Run** the same command — Expected: PASS.
- [ ] **Step 5: Commit** `feat: bepaal externe refs en grootte van een schema`

### Task 2: Bundel als compound document

**Files:**
- Create: `pkg/schema_client/helpers/schemaanalysis/bundle.go`
- Test: `pkg/schema_client/helpers/schemaanalysis/bundle_test.go`

**Interfaces:**
- Consumes: `ExternalRefs`, `BaseURI` (Task 1).
- Produces:
  - `type Lookup func(uri string) (map[string]any, bool)` — zoekt een schema op absolute URI zonder fragment.
  - `func Bundle(content map[string]any, baseURI string, lookup Lookup) (bundle map[string]any, unresolved []Ref)` — diepe kopie van `content`; elk transitief bereikbaar schema wordt eenmaal ingevoegd onder `$defs` (of `definitions` als het root-`$schema` draft-04/06/07 is) met key = zijn absolute URI, en met `$id` gezet op die URI als het geen absolute `$id` had. Bestaande `$defs`-keys blijven. `unresolved` bevat refs (uit root en ingevoegde schema's) waarvan `Target` leeg is of niet gevonden wordt. `$ref`-waarden worden niet gewijzigd.

- [ ] **Step 1: Write the failing tests**
  - `TestBundleEmbedsTransitiveDependencies`: A → B → C via lookup-map → bundel heeft `$defs` met keys URI(B) en URI(C), elk met `$id` gelijk aan de key; A's eigen `$ref` is ongewijzigd; `unresolved` leeg.
  - `TestBundleHandlesCycles`: A → B → A → bundel bevat B één keer en A niet onder `$defs`; test rondt af (gebruik `t.Deadline`-vrije test, geen timeout nodig).
  - `TestBundleUsesDefinitionsForDraft07`: root `$schema` `http://json-schema.org/draft-07/schema#` → embed onder `definitions`.
  - `TestBundleReportsUnresolved`: ref naar onbekende URI → in `unresolved`, bundel bevat geen embed ervoor.
  - `TestBundleDoesNotMutateInput`: originele `content` heeft na afloop geen `$defs`.
- [ ] **Step 2: Run** `go test ./pkg/schema_client/helpers/schemaanalysis/ -run TestBundle` — Expected: FAIL.
- [ ] **Step 3: Implement** `Bundle` with a breadth-first queue and a visited-set keyed on URI (root URI pre-marked visited).
- [ ] **Step 4: Run** — Expected: PASS.
- [ ] **Step 5: Commit** `feat: bundel een schema met zijn dependencies`

### Task 3: Health-regels

**Files:**
- Create: `pkg/schema_client/helpers/schemaanalysis/health.go`
- Test: `pkg/schema_client/helpers/schemaanalysis/health_test.go`

**Interfaces:**
- Consumes: `Ref` (Task 1), `models.SourceMetaHealthIssue`.
- Produces: `func Health(content map[string]any, schemaURL string, unresolved []Ref) (score int, issues []models.SourceMetaHealthIssue)`.

Regels (name — gewicht — wanneer issue — pointers):

| name | gewicht | issue als | pointers |
|---|---|---|---|
| `schema_dialect` | 3 | `$schema` ontbreekt of `jsonschema.NormalizeDialect` geeft `unknown` | `[""]` |
| `id_matches_url` | 3 | `schemaURL` niet leeg en `$id` ≠ `schemaURL` | `["/$id"]` of `[""]` |
| `top_level_title` | 2 | `title` leeg | `[""]` |
| `top_level_description` | 2 | `description` leeg | `[""]` |
| `top_level_examples` | 1 | `examples` geen niet-lege array | `[""]` |
| `property_descriptions` | 2 | een property onder (geneste) `properties` zonder `description` en zonder `$ref` | pointer per property |
| `unresolved_refs` | 3 | `len(unresolved) > 0` | `At` per ref |

Score = round(100 × som gewichten van regels zonder issue / som van alle toepasselijke gewichten); `id_matches_url` is niet toepasselijk bij lege `schemaURL`. `message` in het Nederlands, kort, per regel vast.

- [ ] **Step 1: Write the failing tests**
  - `TestHealthPerfectSchemaScores100`: schema met alles goed → 100, geen issues.
  - `TestHealthReportsMissingExamplesAndPropertyDescription`: alleen examples ontbreken en property `name` zonder description → issues `top_level_examples` (pointers `[""]`) en `property_descriptions` (pointers `["/properties/name"]`), score `round(100*13/16) = 81`.
  - `TestHealthSkipsIdRuleWithoutURL`: geen `$id`, `schemaURL` leeg, verder perfect → 100.
  - `TestHealthReportsUnresolvedRefs`: één unresolved Ref met `At` `/properties/x/$ref` → issue `unresolved_refs` met die pointer.
- [ ] **Step 2: Run** `go test ./pkg/schema_client/helpers/schemaanalysis/ -run TestHealth` — Expected: FAIL.
- [ ] **Step 3: Implement** `Health`, issues in de volgorde van de tabel.
- [ ] **Step 4: Run** — Expected: PASS.
- [ ] **Step 5: Commit** `feat: eigen health-regels voor schemas`

### Task 4: Analyse van één schema

**Files:**
- Create: `pkg/schema_client/helpers/schemaanalysis/analyze.go`
- Test: `pkg/schema_client/helpers/schemaanalysis/analyze_test.go`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces:
  - `type Result struct { Dependencies []models.SourceMetaDependency; DependencyCount int; Health int; HealthIssues []models.SourceMetaHealthIssue; Bytes int; BytesBundled int; BaseDialect string; Dialect string }`
  - `func Analyze(content map[string]any, raw []byte, schemaURL string, lookup Lookup) Result` — base = `BaseURI(content, schemaURL)`; `Dependencies` = één entry per Ref met `From` = base, `To` = `Target` (of `Raw` als leeg), `At`; `DependencyCount` = aantal unieke niet-lege targets; `Bytes` = `CompactBytes(raw)` (bij lege raw: compacte marshal van content); `BytesBundled` = compacte marshal-lengte van de bundel; `BaseDialect` en `Dialect` = de `$schema`-string van content (beide gelijk, getrimd).

- [ ] **Step 1: Write the failing test** `TestAnalyzeApiSchema`: Api → Organisation en ApiLifecycle (lookup-map met beide) → 2 Dependencies met `From` = Api-URL, `DependencyCount` 2, `BytesBundled > Bytes`, `Dialect` `https://json-schema.org/draft/2020-12/schema`.
- [ ] **Step 2: Run** `go test ./pkg/schema_client/helpers/schemaanalysis/ -run TestAnalyze` — Expected: FAIL.
- [ ] **Step 3: Implement** `Analyze`.
- [ ] **Step 4: Run** `go test ./pkg/schema_client/helpers/schemaanalysis/` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: analyseer een schema zonder SourceMeta`

### Task 5: Static harvester en opslaan

**Files:**
- Create: `pkg/schema_client/jobs/static_harvester.go`, `pkg/schema_client/jobs/static_harvester_test.go`
- Delete: `pkg/schema_client/jobs/sourcemeta_harvester.go`, `pkg/schema_client/jobs/sourcemeta_harvester_test.go`
- Modify: `pkg/schema_client/jobs/schema_refresh_job.go`, `schema_refresh_job_test.go`, `pkg/schema_client/models/schema.go` (`SourceMetaSchemaMetadata` weg), `pkg/schema_client/services/schema_service.go` (`HarvestSourceMetaSchemas` en One-helpers weg), `schema_service_test.go`, `pkg/schema_client/schemas/schemas_repository.go` (+ test), `pkg/schema_client/helpers/util/convert.go` indien nodig.

**Interfaces:**
- Produces:
  - `models.StaticSchemaFile{ URL string; Raw []byte }` en `models.StaticHarvest{ Files []StaticSchemaFile; ListedURLs []string; Complete bool }` (in `models/schema.go`).
  - `jobs.NewStaticHarvester(baseURL string, client *http.Client) *StaticHarvester`; `(*StaticHarvester).Harvest(ctx) (models.StaticHarvest, error)`. Leest nginx `autoindex_format json` (`[{"name","type":"directory"|"file",...}]`) recursief vanaf `baseURL` (default `https://static.developer.overheid.nl/schemas/`, env `SCHEMA_STATIC_BASE_URL`); bestanden eindigend op `.json` tellen; `ListedURLs` = alle gevonden bestands-URL's; `Complete` = alle listings gelukt; een bestand dat niet op te halen is komt wel in `ListedURLs`, niet in `Files`. Respecteert max 10 MiB per bestand (zoals `maxSchemaBytes`).
  - `SchemaRefresher.HarvestStaticSchemas(ctx, models.StaticHarvest) (int, error)` vervangt `HarvestSourceMetaSchemas`; `StaticHarvestClient` interface met `Harvest(ctx) (models.StaticHarvest, error)`.
  - `SchemasRepository.DeleteSchema(ctx context.Context, id string) error`.
  - Service: per bestand `jsonschema.ParseValidateAndHash`; schema met `SourceMetaIdentifier` = `SchemaUrl` = bestands-URL, `Title`/`Description` uit content (fallback bestandsnaam zonder `.schema.json`), `SourceMetaName` = bestandsnaam zonder extensie, `SourceMetaPath` = pad relatief aan de base-URL zonder extensie (bv. `don/Api`), `SourceMetaRoot` = eerste segment, `Content`, `Hash`, `Dialect`, `RootType`, `LastCrawledAt`; nieuw record krijgt `shortid`. Ongeldige JSON: loggen, overslaan. Na opslaan, alleen als `Complete`: records met niet-lege `SourceMetaIdentifier` die niet in `ListedURLs` staan → `DeleteSchema`.

- [ ] **Step 1: Write the failing tests**
  - `TestStaticHarvesterWalksDirectories` (httptest-server met listing `/schemas/` → dir `don/`, dir `adr/`; `don/` → `Api.schema.json`, `README.md`; `adr/` → dir `2.2/` → `ProblemDetails.schema.json`) → 2 Files met juiste URL's, `Complete` true.
  - `TestStaticHarvesterMarksIncompleteOnListingError`: subdirectory geeft 500 → `Complete` false, geen error-return voor de hele run.
  - `TestHarvestStaticSchemasStoresFileURL`: Api-bestand → record met `SchemaUrl` en `SourceMetaIdentifier` gelijk aan bestands-URL, `SourceMetaName` `Api`, content-`$id` ongewijzigd.
  - `TestHarvestStaticSchemasDeletesVanishedOnlyWhenComplete`: bestaand record X niet in listing → verwijderd bij `Complete` true, behouden bij `Complete` false.
  - `TestHarvestStaticSchemasKeepsRecordWhenFileInvalid`: bestaand record Y, bestand Y in `ListedURLs` maar raw is `{`-ongeldig → Y blijft, ongewijzigd.
  - Repository: `TestDeleteSchema`.
- [ ] **Step 2: Run** `go test ./pkg/schema_client/...` — Expected: FAIL (compile errors on missing types).
- [ ] **Step 3: Implement** harvester, models, repository-delete, service-methode; wire `NewStaticHarvester(os.Getenv("SCHEMA_STATIC_BASE_URL"), nil)` in de job; verwijder One-code en hun tests (`sourceMetaPublicSchemaURL`, `sourceMetaBundledSchemaURL`, `normalizeSourceMetaPath`, `sourceMetaRootFromPath`, `escapedPath`, `defaultSourceMetaPublicSchemaBaseURL`) waar ze niet meer gebruikt worden; de `RefreshChangedSchemas`-skip op `SourceMetaIdentifier` blijft.
- [ ] **Step 4: Run** `go vet ./... && go test ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: harvest schemas uit static in plaats van SourceMeta One`

### Task 6: Analyse voor alle schema's

**Files:**
- Modify: `pkg/schema_client/services/schema_service.go`, `schema_service_test.go`

**Interfaces:**
- Consumes: `schemaanalysis.Analyze`, `schemaanalysis.Lookup` (Task 4); bestaande `enrichSourceMetaDependencies`.
- Produces: `func (s *SchemaService) RefreshAnalysis(ctx context.Context) error` — laadt `AllSchemas`, bouwt een lookup op `SourceMetaIdentifier`, `SchemaUrl` en absolute content-`$id` (zonder fragment), draait `Analyze` per schema (schemaURL = `SchemaUrl`; raw = `json.Marshal(Content)`), zet `SourceMetaDependencyDetails`, `SourceMetaDependencies` (= `DependencyCount`), `SourceMetaHealth`, `SourceMetaHealthIssues`, `SourceMetaBytes`, `SourceMetaBytesBundled`, `SourceMetaBaseDialect`, `SourceMetaDialect` en verrijkt met `enrichSourceMetaDependencies` (lookup op dezelfde sleutels, zodat `toSchemaId` ook via `SchemaUrl` gevonden wordt); slaat alleen op en publiceert naar Typesense als iets veranderde. Wordt aangeroepen aan het eind van `HarvestStaticSchemas`, `CreateSchemaFromInput`, `applySchemaUpdate` en `RefreshChangedSchemas` (fout: loggen, niet de request laten falen).

- [ ] **Step 1: Write the failing tests**
  - `TestRefreshAnalysisLinksDependenciesAcrossSources`: geharvest Organisation + via POST-pad opgeslagen schema met absolute `$ref` naar Organisation's URL → dependency met `ToSchemaId` = id van Organisation, `SourceMetaDependencies` 1, health > 0.
  - `TestRefreshAnalysisReportsUnresolvedForBodyWithoutId`: schema zonder `$id` en zonder `SchemaUrl` met `$ref: "Other.schema.json"` → issue `unresolved_refs`, geen panic.
  - `TestCreateSchemaRunsAnalysis`: na `CreateSchemaFromInput` (schemaBody) heeft het record een `SourceMetaHealth` > 0.
- [ ] **Step 2: Run** `go test ./pkg/schema_client/services/ -run 'TestRefreshAnalysis|TestCreateSchemaRunsAnalysis'` — Expected: FAIL.
- [ ] **Step 3: Implement** `RefreshAnalysis` en de vier aanroepen.
- [ ] **Step 4: Run** `go test ./...` — Expected: PASS.
- [ ] **Step 5: Commit** `feat: analyseer alle schemas in het register`

### Task 7: Bundel-endpoint

**Files:**
- Modify: `pkg/schema_client/services/schema_service.go`, `pkg/schema_client/handler/schema_handler.go`, `pkg/schema_client/routers.go`, `api/openapi.json`, en de bijbehorende `_test.go`'s.

**Interfaces:**
- Consumes: `schemaanalysis.Bundle` (Task 2), de lookup-bouw uit Task 6 (deel die als `func (s *SchemaService) analysisLookup(ctx) (schemaanalysis.Lookup, error)`).
- Produces:
  - `func (s *SchemaService) BundleSchema(ctx context.Context, id string) (map[string]any, error)` — `nil, nil` bij onbekend id.
  - Handler `RetrieveSchemaBundle` op `g.GET("/v1/schemas/:id/bundle.json", ...)`, zelfde stijl als `RetrieveSchemaContent` (404 problem bij onbekend, `application/schema+json`).
  - `RefreshAnalysis` zet ook `SourceMetaBundledUrl` = env `SCHEMA_BUNDLE_URL_TEMPLATE` met `{id}` vervangen; default `https://schemas.developer.overheid.nl/schemas/download/{id}/bundle.json`.
  - `api/openapi.json`: pad `/schemas/{id}/bundle.json`, gemodelleerd naar `/schemas/{id}/schema.json`.

- [ ] **Step 1: Write the failing tests**
  - Handler-test `TestRetrieveSchemaBundle`: bestaand schema met dependency → 200, content-type `application/schema+json`, body bevat `$defs` met de dependency-URI; onbekend id → 404 `application/problem+json`.
  - Service-test `TestRefreshAnalysisSetsBundledURL`: `SCHEMA_BUNDLE_URL_TEMPLATE` = `https://x/{id}/b.json` → `SourceMetaBundledUrl` = `https://x/<id>/b.json`.
- [ ] **Step 2: Run** `go test ./pkg/schema_client/... -run 'Bundle|BundledURL'` — Expected: FAIL.
- [ ] **Step 3: Implement** service, handler, route en OAS-pad.
- [ ] **Step 4: Run** `go test ./...` — Expected: PASS; en `jq . api/openapi.json >/dev/null` — Expected: geen output.
- [ ] **Step 5: Commit** `feat: serveer de gebundelde versie van een schema`

### Task 8: Documentatie en changelog

**Files:**
- Modify: `README.md` (sectie "Dagelijkse schema-refresh": One-variabelen vervangen door `SCHEMA_STATIC_BASE_URL` en `SCHEMA_BUNDLE_URL_TEMPLATE`, uitleg dat static een JSON directory-listing moet serveren)
- Create: `.changes/unreleased/Changed-20261104-150000.yaml` (`kind: Changed`, Nederlandse body: register leest schemas rechtstreeks uit static en berekent dependencies, health en bundel zelf; SourceMeta One is niet meer nodig)

- [ ] **Step 1: Update** README en maak het fragment.
- [ ] **Step 2: Run** `gofmt -l . && go vet ./... && go test ./... && go build ./...` — Expected: geen gofmt-output, alles PASS.
- [ ] **Step 3: Run** `grep -rniE "sourcemeta one|SOURCEMETA_ONE|source-meta-svc" --include=*.go --include=*.md . | grep -v docs/superpowers` — Expected: geen treffers.
- [ ] **Step 4: Commit** `docs: beschrijf de harvest uit static`
