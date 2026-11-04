package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

const (
	defaultStaticBaseURL = "https://static.developer.overheid.nl/schemas/"
	maxStaticSchemaBytes = 10 << 20 // 10 MiB, gelijk aan maxSchemaBytes in helpers/jsonschema
	maxStaticDepth       = 20
)

// StaticHarvester leest de originele schemabestanden uit don-static via de
// JSON directory-listing van nginx (autoindex_format json).
type StaticHarvester struct {
	baseURL    string
	httpClient *http.Client
}

type staticListEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// NewStaticHarvester maakt een harvester; een lege baseURL gebruikt de default.
func NewStaticHarvester(baseURL string, httpClient *http.Client) *StaticHarvester {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = defaultStaticBaseURL
	}
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &StaticHarvester{baseURL: baseURL, httpClient: httpClient}
}

// Harvest loopt recursief door de directory-listings en haalt alle .json-bestanden op.
// Mislukte listings maken het resultaat onvolledig (Complete=false) maar breken de run niet af.
func (h *StaticHarvester) Harvest(ctx context.Context) (models.StaticHarvest, error) {
	result := models.StaticHarvest{BaseURL: h.baseURL, Complete: true}
	if _, err := url.Parse(h.baseURL); err != nil {
		return result, err
	}
	h.walk(ctx, h.baseURL, 0, &result)
	return result, ctx.Err()
}

func (h *StaticHarvester) walk(ctx context.Context, dirURL string, depth int, result *models.StaticHarvest) {
	if ctx.Err() != nil {
		result.Complete = false
		return
	}
	if depth > maxStaticDepth {
		log.Printf("[static] maximale diepte bereikt bij %s", dirURL)
		result.Complete = false
		return
	}
	entries, err := h.fetchListing(ctx, dirURL)
	if err != nil {
		log.Printf("[static] listing ophalen mislukt: %v", err)
		result.Complete = false
		return
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" || strings.Contains(name, "/") || name == "." || name == ".." {
			continue
		}
		entryURL := dirURL + url.PathEscape(name)
		switch entry.Type {
		case "directory":
			h.walk(ctx, entryURL+"/", depth+1, result)
		case "file":
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			result.ListedURLs = append(result.ListedURLs, entryURL)
			raw, err := h.fetchFile(ctx, entryURL)
			if err != nil {
				log.Printf("[static] bestand ophalen mislukt: %v", err)
				continue
			}
			result.Files = append(result.Files, models.StaticSchemaFile{URL: entryURL, Raw: raw})
		}
	}
}

func (h *StaticHarvester) fetchListing(ctx context.Context, listURL string) ([]staticListEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", listURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("listing %s gaf status %s", listURL, resp.Status)
	}
	var entries []staticListEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("listing %s parsen mislukt: %w", listURL, err)
	}
	return entries, nil
}

func (h *StaticHarvester) fetchFile(ctx context.Context, fileURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/schema+json, application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fileURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s gaf status %s", fileURL, resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxStaticSchemaBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s lezen mislukt: %w", fileURL, err)
	}
	if len(raw) > maxStaticSchemaBytes {
		return nil, fmt.Errorf("%s is groter dan %d bytes", fileURL, maxStaticSchemaBytes)
	}
	return raw, nil
}
