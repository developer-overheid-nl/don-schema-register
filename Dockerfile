FROM ghcr.io/sourcemeta/one:v6.1.0

COPY one.json .
COPY schemas ./schemas

RUN sourcemeta one.json
