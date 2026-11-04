# Eigen schema-analyse in plaats van SourceMeta One

## Waarom

Het schema-register haalt de schema's uit `don-static` nu op via SourceMeta One.
One herschrijft wat het serveert: bestandsnamen worden lowercase en verliezen
`.schema.json` (`Api.schema.json` wordt `api`), en `$id` en `$ref`s worden
herschreven naar `https://schemas.developer.overheid.nl/schemas/...`. Die
identifiers geven een 404. Het register toont daardoor niet de bestanden die in
`don-static` staan, en de bestandsnaam is niet terug te rekenen (`agendaevent`
was `AgendaEvent`). One heeft daar geen instelling voor.

One is bovendien source-available (BUSL-1.1, commercieel voor "hosted or managed
service"), niet open source. We willen volledig onafhankelijk worden van
SourceMeta: geen One-server, geen `jsonschema`-CLI, geen Sourcemeta-libraries.

## Doelen

- Het register leest de originele bestanden uit `don-static`. `schemaUrl` is de
  echte URL van het bestand (`https://static.developer.overheid.nl/schemas/don/Api.schema.json`),
  met de bestandsnaam en `$id` zoals ze in de repo staan.
- Alles wat de detailpagina nu toont blijft werken, berekend door de backend zelf:
  - dependencies (uitgaand) en dependents (inkomend), met titel en link naar het
    schema in het register;
  - dialect en base dialect;
  - grootte van het schema en van de gebundelde versie;
  - een gebundelde versie van het schema, te downloaden;
  - een health-score met verbeterpunten.
- De API-contracten blijven in deze fase gelijk (zelfde velden, zelfde vorm), zodat
  de schema-site zonder wijziging blijft werken.

## Niet-doelen (deze fase)

- De definitieve health-regels. Die worden later een eigen "ADR-checker voor
  schemas". Nu volstaat een eenvoudige eigen regelset (een aftreksel).
- Het hernoemen van de `sourceMeta*`-velden in API en database. Dat gebeurt later,
  samen met de site.
- Het opruimen van One in `don-infra` en `don-static` en het aanpassen van de site.
  Dat zijn vervolgstappen in andere repositories (zie onderaan).

## Anti-doelen

- Geen afhankelijkheid van SourceMeta in welke vorm dan ook.
- Geen herschrijving van `$id` of `$ref`: de inhoud die we opslaan en tonen is het
  originele bestand.
- Een harvest die halverwege mislukt mag geen schema's uit het register laten
  verdwijnen.

## Constraints

- Go-backend `don-schema-register` (Gin/Fizz/GORM, Postgres). Database-kolommen
  ontstaan via GORM AutoMigrate (`don-register-common`); kolommen worden nooit
  gedropt.
- De bestaande dagelijkse job (`SchemaRefreshJob`, start direct en daarna 07:00)
  blijft de plek waar geharvest wordt.
- Schema's die via de API (POST/PUT) zijn geregistreerd lopen via
  `RefreshChangedSchemas`; die blijven werken zoals nu.
- `api/openapi.json` moet de API blijven beschrijven; de OAS moet ADR-score 100
  houden (zie don-tools).
- Eén changie-fragment voor deze wijziging.

## Wat besloten is

- **Bron van de lijst met schema's:** de backend loopt de mappen onder
  `https://static.developer.overheid.nl/schemas/` door. Hoe die lijst er komt is
  een keuze van de bouwer (zie hieronder); de bestanden zelf worden altijd van
  hun originele URL gehaald.
- **Identiteit van een geharvest schema:** de bestands-URL in static. Die waarde
  gaat in het bestaande veld `sourceMetaIdentifier`, zodat `SaveSchema` het
  bestaande record terugvindt, ook als de `$id` in het bestand ooit afwijkt.
- **`schemaUrl`:** de URL van het bestand in static.
- **Dependencies:** alle `$ref`s die niet met `#` beginnen, geresolved tegen de
  `$id` (of bestands-URL) van het schema. Opgeslagen in
  `sourceMetaDependencyDetails` met `from`, `to` en `at` (JSON-pointer naar de
  `$ref`), en daarna verrijkt met id, link en titel zoals nu
  (`enrichSourceMetaDependencies`). Dependents zijn de dependencies van andere
  schema's die naar dit schema wijzen, net als nu.
- **Health:** eigen regels, eigen score 0–100. Verbeterpunten in hetzelfde
  formaat als nu (`name`, `message`, `description`, `pointers`).
- **Bundel:** de backend maakt de bundel zelf: alle externe `$ref`s (ook
  transitief) worden ingevoegd onder `$defs` en de `$ref`s worden daarnaartoe
  herschreven. De bundel wordt geserveerd via een nieuw endpoint
  `GET /v1/schemas/{id}/bundle.json`, naast het bestaande `schema.json`-endpoint.

## Wat aan de bouwer is

- Hoe de lijst met bestanden wordt opgehaald. Voorkeur: nginx in `don-static`
  serveert voor `/schemas/` een JSON directory-listing (`autoindex_format json`),
  zodat de lijst altijd gelijk is aan wat er live staat. De backend moet ook
  werken tegen een alternatieve bron via een omgevingsvariabele.
- De precieze health-regels en hun gewicht, zolang ze in elk geval dekken:
  geldige `$schema`, `$id` gelijk aan de bestands-URL, `title`, `description`,
  top-level `examples`, beschrijvingen bij properties, en `$ref`s die resolven.
- Hoe het pakket voor analyse is opgedeeld, en de naam van de nieuwe
  omgevingsvariabelen. `SOURCEMETA_ONE_API_BASE` en de One-harvester verdwijnen.
- Welke publieke URL in `sourceMetaBundledUrl` komt. De site toont die als
  kopieerbare link; de API zelf vraagt een API-key. Voorkeur: een configureerbaar
  sjabloon, met als standaard het download-pad van de schema-site
  (`/schemas/download/{id}/bundle.json`), dat in de vervolgstap in de site komt.
- Opruimen van geharveste records die niet meer in static staan: alleen na een
  volledig geslaagde harvest, en alleen records met een `sourceMetaIdentifier`.

## Keuzes van de bouwer (na review)

- **Refs resolven zonder netwerk.** In `don-static` (main) is de `$id` al de
  bestands-URL (`.../don/Api.schema.json`) en verwijzen `$ref`s naar
  bestandsnamen (`ApiLifecycle.schema.json`). Een `$ref` wordt geresolved tegen
  de `$id` en opgezocht onder alle schema's in het register (op
  `sourceMetaIdentifier`, `schemaUrl` en `$id` in de content). Er wordt niets
  extra over het netwerk opgehaald. Een `$ref` die niet resolvet blijft staan
  als dependency zonder doel, blijft ongewijzigd in de bundel en levert een
  health-issue op.
- **Analyse voor alle schema's.** Dependencies, health, bytes en bundel worden
  ook berekend voor schema's die via POST/PUT zijn geregistreerd, met dezelfde
  regels. `bundle.json` werkt dus voor elk schema.
- **Id's mogen veranderen.** Bestaande records met een identifier van One
  worden niet gemigreerd; een nieuwe harvest maakt nieuwe records aan en het
  opruimen haalt de oude weg. Prod is op 2026-11-04 geleegd, dus daar speelt dit
  niet.
- **Opruimen na harvest.** De harvest geldt als geslaagd als de listing
  volledig is opgehaald. Alleen geharveste records waarvan de bestands-URL niet
  meer in de listing voorkomt worden verwijderd. Een bestand dat in de listing
  staat maar niet op te halen of te parsen is, houdt zijn bestaande record en
  wordt gelogd. Mislukt het ophalen van de listing, dan wordt niets verwijderd.

## Vervolgstappen in andere repositories (niet in deze branch)

1. **don-static:** JSON directory-listing voor `/schemas/`; `Dockerfile.schemas`,
   `one.json` en de One-stap in CI weghalen; README bijwerken.
2. **don-infra:** in de static-HTTPRoute alle `/schemas`-verkeer naar nginx; de
   `source-meta`-deployment en -service weghalen; env van de schema-backend bijwerken.
3. **don-register-site:** download-route voor de bundel; label "SourceMeta dialect"
   en vergelijkbare teksten aanpassen; later de veldnamen.
