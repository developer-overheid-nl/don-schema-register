# don-schema-register

A Docker-based JSON Schema validation register for testing schemas.

## Features

- Docker image with JSON Schema validation capabilities
- GitHub Actions workflow for automated building
- Example schema included for testing
- Python-based validation using the `jsonschema` library

## Quick Start

### Build the Docker Image

```bash
docker build -t don-schema-register:test .
```

### Run Schema Validation

```bash
docker run --rm don-schema-register:test
```

### Validate Custom Schemas

```bash
docker run --rm -v /path/to/your/schemas:/workspace/schemas don-schema-register:test \
  python3 /workspace/validate.py /workspace/schemas/your-schema.json /workspace/test-data.json
```

## GitHub Actions

The repository includes a GitHub Actions workflow (`.github/workflows/docker-build.yml`) that automatically builds the Docker image on push or pull request events.

## Files

- `Dockerfile` - Docker image definition
- `validate.py` - Python script for JSON Schema validation
- `schemas/example-schema.json` - Example JSON Schema
- `.github/workflows/docker-build.yml` - GitHub Actions workflow

## References

- JSON Schema: https://json-schema.org/
- Python jsonschema library: https://python-jsonschema.readthedocs.io/