FROM ghcr.io/sourcemeta/one:latest

COPY one.json .
COPY schemas ./schemas

RUN sourcemeta one.json
