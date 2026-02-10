import json
import sys
from jsonschema import validate, ValidationError

def validate_json(schema_file, data_file):
    """Validate a JSON document against a JSON Schema"""
    with open(schema_file) as sf:
        schema = json.load(sf)
    with open(data_file) as df:
        data = json.load(df)
    try:
        validate(instance=data, schema=schema)
        print("✓ Validation successful!")
        return 0
    except ValidationError as e:
        print(f"✗ Validation failed: {e.message}")
        return 1

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("Usage: python3 validate.py <schema_file> <data_file>")
        sys.exit(1)
    sys.exit(validate_json(sys.argv[1], sys.argv[2]))
