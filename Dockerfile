# Use Ubuntu as base for better compatibility
FROM ubuntu:22.04

# Set non-interactive mode
ENV DEBIAN_FRONTEND=noninteractive

# Install Python 3 (already included in Ubuntu base)
RUN apt-get update && \
    apt-get install -y python3 python3-pip && \
    rm -rf /var/lib/apt/lists/*

# Install jsonschema package with pinned version
RUN pip3 install jsonschema==4.26.0

# Create working directory
WORKDIR /workspace

# Copy validation script
COPY validate.py /workspace/validate.py

# Copy schemas
COPY schemas /workspace/schemas

# Create a test JSON document
RUN echo '{"firstName": "John", "lastName": "Doe", "age": 30}' > /workspace/test-data.json

# Default command: validate the test data against the schema
CMD ["python3", "/workspace/validate.py", "/workspace/schemas/example-schema.json", "/workspace/test-data.json"]
