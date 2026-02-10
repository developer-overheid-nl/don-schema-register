import json
import sys
from jsonschema import validate, ValidationError

def validate_json(schema_file, data_file):
    """Validate a JSON document against a JSON Schema"""
    try:
        with open(schema_file) as sf:
            schema = json.load(sf)
    except FileNotFoundError:
        print(f"✗ Error: Schema file '{schema_file}' not found")
        return 1
    except json.JSONDecodeError as e:
        print(f"✗ Error: Invalid JSON in schema file '{schema_file}': {e}")
        return 1
    
    try:
        with open(data_file) as df:
            data = json.load(df)
    except FileNotFoundError:
        print(f"✗ Error: Data file '{data_file}' not found")
        return 1
    except json.JSONDecodeError as e:
        print(f"✗ Error: Invalid JSON in data file '{data_file}': {e}")
        return 1
    
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
