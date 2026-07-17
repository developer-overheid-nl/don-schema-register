package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
)

const defaultSourceMetaOneAPIBase = "https://static.don.projects.digilab.network/schemas/"

type SourceMetaHarvester struct {
	baseURL    string
	httpClient *http.Client
}

type sourceMetaListResponse struct {
	Entries []sourceMetaEntry `json:"entries"`
}

type sourceMetaEntry struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Path         string `json:"path"`
	Identifier   string `json:"identifier"`
	Bytes        int    `json:"bytes"`
	BytesBundled int    `json:"bytesBundled"`
	BaseDialect  string `json:"baseDialect"`
	Dialect      string `json:"dialect"`
	Health       int    `json:"health"`
	Dependencies int    `json:"dependencies"`
	Description  string `json:"description"`
}

func NewSourceMetaHarvester(baseURL string, httpClient *http.Client) *SourceMetaHarvester {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultSourceMetaOneAPIBase
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &SourceMetaHarvester{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (h *SourceMetaHarvester) Harvest(ctx context.Context) ([]models.SourceMetaSchemaMetadata, error) {
	rootURL, err := sourceMetaListURL(h.baseURL)
	if err != nil {
		return nil, err
	}

	var schemas []models.SourceMetaSchemaMetadata
	seen := map[string]bool{}
	if err := h.harvestList(ctx, rootURL, seen, &schemas); err != nil {
		return nil, err
	}
	return schemas, nil
}

func (h *SourceMetaHarvester) harvestList(
	ctx context.Context,
	listURL string,
	seen map[string]bool,
	schemas *[]models.SourceMetaSchemaMetadata,
) error {
	if seen[listURL] {
		return nil
	}
	seen[listURL] = true

	response, err := h.fetchList(ctx, listURL)
	if err != nil {
		return err
	}

	for _, entry := range response.Entries {
		switch entry.Type {
		case "schema":
			*schemas = append(*schemas, entry.toMetadata())
		case "directory":
			nextURL, err := sourceMetaDirectoryURL(listURL, entry.Path)
			if err != nil {
				return err
			}
			if err := h.harvestList(ctx, nextURL, seen, schemas); err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *SourceMetaHarvester) fetchList(ctx context.Context, listURL string) (*sourceMetaListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SourceMeta One API ophalen mislukt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("SourceMeta One API gaf status %s voor %s", resp.Status, listURL)
	}

	var response sourceMetaListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("SourceMeta One API response parsen mislukt: %w", err)
	}

	return &response, nil
}

func (e sourceMetaEntry) toMetadata() models.SourceMetaSchemaMetadata {
	return models.SourceMetaSchemaMetadata{
		Name:         e.Name,
		Identifier:   e.Identifier,
		Bytes:        e.Bytes,
		BytesBundled: e.BytesBundled,
		BaseDialect:  e.BaseDialect,
		Dialect:      e.Dialect,
		Health:       e.Health,
		Dependencies: e.Dependencies,
		Description:  e.Description,
	}
}

func sourceMetaListURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	u.Path = path.Join(u.Path, "self", "v1", "api", "list")
	return u.String(), nil
}

func sourceMetaDirectoryURL(currentListURL, directoryPath string) (string, error) {
	u, err := url.Parse(currentListURL)
	if err != nil {
		return "", err
	}
	listRoot := u.Path
	if idx := strings.Index(listRoot, "/self/v1/api/list"); idx >= 0 {
		listRoot = listRoot[:idx+len("/self/v1/api/list")]
	}
	u.Path = path.Join(listRoot, strings.TrimPrefix(directoryPath, "/"))
	if strings.HasSuffix(directoryPath, "/") {
		u.Path += "/"
	}
	return u.String(), nil
}
