# Schema-register API

API van het Schema register (schemas.developer.overheid.nl)

## Overview

- API version: 1.0.0
- Runtime: Go
- Database: PostgreSQL

Deze API registreert en ontsluit metadata over JSON Schemas van
overheidsorganisaties. Nieuwe schemas worden opgehaald of uit de request body
gelezen, gevalideerd, van metadata voorzien en opgeslagen in PostgreSQL.

## Lokaal draaien

1. Start de afhankelijkheden:

   ```bash
   docker compose up -d
   ```

2. Maak een `.env` bestand aan met minimaal:

   ```env
   DB_HOSTNAME=localhost
   DB_USERNAME=don
   DB_PASSWORD=don
   DB_DBNAME=don_schema_v1
   DB_SCHEMA=public
   ```

3. Start de server:

   ```bash
   go run cmd/main.go
   ```

   De API luistert standaard op poort **1337**.

## Typesense integratie

Nieuwe schemas worden na een succesvolle POST of PUT ook naar Typesense
gestuurd, zodat ze vindbaar zijn in de zoekfunctie. Stel hiervoor de volgende
omgevingsvariabelen in:

- `TYPESENSE_ENDPOINT`: basis-URL van de Typesense cluster, bijvoorbeeld
  `https://search.don.apps.digilab.network`.
- `TYPESENSE_API_KEY`: API key met schrijfrechten.
- `TYPESENSE_COLLECTION`: naam van de collectie, standaard `schema-register`.
- `TYPESENSE_DETAIL_BASE_URL`: basis-URL voor detailpagina's in de frontend,
  standaard `https://schemas.developer.overheid.nl/schemas`.
- `TYPESENSE_LANGUAGE`: taalcode voor indexdocumenten, standaard `nl`.
- `TYPESENSE_ITEM_PRIORITY`: prioriteit in de zoekindex, standaard `1`.
- `TYPESENSE_DEFAULT_TAGS`: komma-gescheiden tags, standaard
  `schema-register,schema`.
- `ENABLE_TYPESENSE`: zet op `false` om Typesense indexing volledig uit te
  schakelen, standaard `true`.

## Dagelijkse schema-refresh

Bij het opstarten van de server wordt automatisch een aparte service gestart
die direct een refresh-run uitvoert. Daarna draait de job iedere ochtend om
**07:00** en haalt alle geregistreerde schemas met een `schemaUrl` opnieuw op.
Zodra de inhoud is gewijzigd, volgen dezelfde stappen als bij een POST:
validatie, metadata bijwerken, opslaan en opnieuw indexeren in Typesense.

SourceMeta schemas worden ook tijdens deze job geharvest. Relevante
omgevingsvariabelen:

- `SOURCEMETA_ONE_API_BASE`: interne SourceMeta One API base, standaard
  `http://source-meta-svc:8000/schemas/`.
- `SOURCEMETA_PUBLIC_SCHEMA_BASE_URL`: publieke base-URL voor daadwerkelijke
  schemas, standaard `https://static.developer.overheid.nl/schemas/`.

## Database

De applicatie gebruikt PostgreSQL. De docker-compose start lokaal een Postgres
container met:

- Host: `localhost`
- Port: `5432`
- Username: `don`
- Password: `don`
- Database: `don_schema_v1`

De applicatie voert bij het opstarten de Gorm migraties uit voor organisaties en
schemas.

## Changelog (Changie)

Voor user-facing wijzigingen (fix/feature/breaking) verwachten we per PR een
Changie-fragment in `.changes/unreleased`.

Eenmalig installeren:

```bash
go install github.com/miniscruff/changie@latest
```

Fragment aanmaken:

```bash
changie new
```

Normaal is een fragment niet nodig voor interne refactors zonder zichtbaar
effect, docs-only wijzigingen en CI-only tweaks.

Bij een release kun je de fragments bundelen in `CHANGELOG.md`:

```bash
changie batch <version>
```

Dit gebeurt ook automatisch bij elke merge naar `main` via GitHub Actions:
`changie batch auto` en daarna `changie merge`, waarna automatisch een PR met de
changelog-updates wordt aangemaakt.

## Deployen

De deployment van deze API verloopt via GitHub Actions en een aparte infra
repository.

### Benodigde variabelen en secrets

- Organization variable `INFRA_REPO`, bijvoorbeeld
  `developer-overheid-nl/don-infra`.
- Repository variable `KUSTOMIZE_PATH`, met als basispad bijvoorbeeld
  `apps/api/overlays/`.
- Secrets `RELEASE_PROCES_APP_ID` en `RELEASE_PROCES_APP_PRIVATE_KEY` voor het
  aanpassen van de infra repository.

### Deploy naar test

De testdeploy draait via `.github/workflows/deploy-test.yml`.

- De workflow draait op pushes naar branches behalve `main`.
- Alleen commits met `[deploy-test]` in de commit message worden echt gedeployed.
- Er wordt een image gebouwd en gepusht naar `ghcr.io/<owner>/<repo>` met tags
  `test` en de commit SHA.
- Daarna wordt in `INFRA_REPO` het bestand
  `${KUSTOMIZE_PATH}test/kustomization.yaml` bijgewerkt naar de nieuwe image tag
  en direct gecommit.

Voorbeeld commit message:

```text
feat: pas schema registratie aan [deploy-test]
```

### Deploy naar productie

De productiedeploy draait via `.github/workflows/deploy-prod.yml`.

- De workflow draait bij een push naar `main`.
- Er wordt een image gebouwd en gepusht naar `ghcr.io/<owner>/<repo>` met tags
  `latest` en de commit SHA.
- Er wordt in `INFRA_REPO` een release branch aangemaakt of bijgewerkt.
- In `${KUSTOMIZE_PATH}prod/kustomization.yaml` wordt de image tag bijgewerkt
  naar de commit SHA van deze repository.
- Daarna wordt automatisch een pull request in de infra repository geopend of
  bijgewerkt.
- De productie-uitrol gebeurt door die pull request te mergen.

### Contributies en deploy

Een contribution of pull request leidt niet automatisch tot een deployment.

- Een pull request triggert wel CI, waaronder tests, linting en een Docker build
  als controle.
- De CI-build pusht geen image naar GHCR en past de infra repository niet aan
  voor pull requests.
- Er is geen automatische preview-omgeving per pull request.
- Een testdeploy gebeurt pas na een push naar een branch in deze repository met
  `[deploy-test]` in de commit message.

## Licentie

Copyright (c) Geonovum. Licensed under the EUPL-1.2.

Zie ook [publiccode.yml](publiccode.yml) voor projectmetadata.
