# DON Schema Register

Dit repository bevat een minimale setup om JSON Schema-documentatie te genereren en te serveren met [Sourcemeta One](https://one.sourcemeta.com/).

## Structuur

- `one.json`: configuratie voor Sourcemeta One
- `schemas/`: JSON Schema-bestanden (voorbeeld: `schemas/person.json`)
- `Dockerfile`: bouwt een image die de documentatie genereert en serveert
- `.github/workflows/ci-cd.yml`: CI/CD pipeline voor build + push naar GHCR

## Lokaal gebruiken

### 1) Image bouwen

```bash
docker build -t don-schema-register .
```

### 2) Container starten

```bash
docker run --rm -p 8000:8000 don-schema-register
```

Open daarna:

- http://localhost:8000

## CI/CD

De workflow in `.github/workflows/ci-cd.yml` doet het volgende:

1. Valideert de Docker build (`validate-image`)
2. Bouwt en pusht de image naar GHCR (`build-and-push`)

Tags die worden gepusht:

- `ghcr.io/<owner>/<repo>:latest`
- `ghcr.io/<owner>/<repo>:<git-sha>`
