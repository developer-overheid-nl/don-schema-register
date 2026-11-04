/*
 * Schema register API v1
 *
 * API van het Schema register (schemas.developer.overheid.nl)
 *
 * API version: 1.0.0
 * Contact: developer.overheid@geonovum.nl
 */

package models

import (
	"time"

	commonfilters "github.com/developer-overheid-nl/don-register-common/filters"
	commonpagination "github.com/developer-overheid-nl/don-register-common/pagination"
)

type Contact struct {
	Name  string `json:"name"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

type SchemaSummary struct {
	Id               string               `json:"id" gorm:"column:id;primaryKey"`
	SchemaUrl        string               `json:"schemaUrl,omitempty" gorm:"column:schema_url"`
	Title            string               `json:"title,omitempty" gorm:"column:title"`
	Description      string               `json:"description,omitempty" gorm:"column:description"`
	Dialect          string               `json:"dialect,omitempty" gorm:"column:dialect"`
	RootType         string               `json:"rootType,omitempty" gorm:"column:root_type"`
	Collection       string               `json:"collection,omitempty" gorm:"column:collection"`
	Contact          Contact              `json:"contact" gorm:"-"`
	Organisation     *OrganisationSummary `json:"organisation,omitempty" gorm:"foreignKey:OrganisationID;references:Uri"`
	OrganisationID   *string              `json:"-" gorm:"column:organisation_id"`
	FileName         string               `json:"fileName,omitempty" gorm:"column:file_name"`
	Path             string               `json:"path,omitempty" gorm:"column:path"`
	BundledSchemaUrl string               `json:"bundledSchemaUrl,omitempty" gorm:"column:bundled_schema_url"`
	Bytes            int                  `json:"bytes,omitempty" gorm:"column:bytes"`
	BundledBytes     int                  `json:"bundledBytes,omitempty" gorm:"column:bundled_bytes"`
	DialectUri       string               `json:"dialectUri,omitempty" gorm:"column:dialect_uri"`
	Health           int                  `json:"health,omitempty" gorm:"column:health"`
	DependencyCount  int                  `json:"dependencyCount,omitempty" gorm:"column:dependency_count"`
	CreatedAt        time.Time            `json:"createdAt" gorm:"column:created_at"`
	LastCrawledAt    time.Time            `json:"lastCrawledAt" gorm:"column:last_crawled_at"`
}

type SchemaDetail struct {
	SchemaSummary
	Content      map[string]any `json:"content,omitempty"`
	Dependencies []Dependency   `json:"dependencies,omitempty"`
	HealthIssues []HealthIssue  `json:"healthIssues,omitempty"`
}

type Schema struct {
	Id               string         `json:"id" gorm:"column:id;primaryKey"`
	SchemaUrl        string         `json:"schemaUrl,omitempty" gorm:"column:schema_url"`
	Title            string         `json:"title" gorm:"column:title"`
	Description      string         `json:"description,omitempty" gorm:"column:description"`
	Dialect          string         `json:"dialect,omitempty" gorm:"column:dialect"`
	RootType         string         `json:"rootType,omitempty" gorm:"column:root_type"`
	Collection       string         `json:"collection,omitempty" gorm:"column:collection"`
	ContactName      string         `json:"contact_name,omitempty" gorm:"column:contact_name"`
	ContactUrl       string         `json:"contact_url,omitempty" gorm:"column:contact_url"`
	ContactEmail     string         `json:"contact_email,omitempty" gorm:"column:contact_email"`
	Organisation     *Organisation  `json:"-" gorm:"foreignKey:OrganisationID;references:Uri"`
	OrganisationID   *string        `json:"-" gorm:"column:organisation_id"`
	Content          map[string]any `json:"content,omitempty" gorm:"column:content;serializer:json"`
	Hash             string         `json:"-" gorm:"column:schema_hash"`
	FileName         string         `json:"fileName,omitempty" gorm:"column:file_name"`
	Path             string         `json:"path,omitempty" gorm:"column:path"`
	BundledSchemaUrl string         `json:"bundledSchemaUrl,omitempty" gorm:"column:bundled_schema_url"`
	SourceUrl        string         `json:"-" gorm:"column:source_url"`
	Bytes            int            `json:"bytes,omitempty" gorm:"column:bytes"`
	BundledBytes     int            `json:"bundledBytes,omitempty" gorm:"column:bundled_bytes"`
	DialectUri       string         `json:"dialectUri,omitempty" gorm:"column:dialect_uri"`
	Health           int            `json:"health,omitempty" gorm:"column:health"`
	HealthIssues     []HealthIssue  `json:"healthIssues,omitempty" gorm:"column:health_issues;serializer:json"`
	DependencyCount  int            `json:"dependencyCount,omitempty" gorm:"column:dependency_count"`
	Dependencies     []Dependency   `json:"dependencies,omitempty" gorm:"column:dependencies;serializer:json"`
	CreatedAt        time.Time      `json:"createdAt" gorm:"column:created_at"`
	LastCrawledAt    time.Time      `json:"lastCrawledAt" gorm:"column:last_crawled_at"`
}

type Dependency struct {
	From            string `json:"from"`
	To              string `json:"to"`
	At              string `json:"at"`
	FromSchemaId    string `json:"fromSchemaId,omitempty"`
	FromSchemaUrl   string `json:"fromSchemaUrl,omitempty"`
	FromSchemaTitle string `json:"fromSchemaTitle,omitempty"`
	ToSchemaId      string `json:"toSchemaId,omitempty"`
	ToSchemaUrl     string `json:"toSchemaUrl,omitempty"`
	ToSchemaTitle   string `json:"toSchemaTitle,omitempty"`
}

type HealthIssue struct {
	Name        string   `json:"name"`
	Message     string   `json:"message"`
	Description string   `json:"description,omitempty"`
	Pointers    []string `json:"pointers"`
}

// StaticSchemaFile is een uit don-static opgehaald schemabestand.
type StaticSchemaFile struct {
	URL string
	Raw []byte
}

// StaticHarvest is het resultaat van een run over de don-static directory-listing.
// ListedURLs bevat alle gevonden bestands-URL's (ook niet-ophaalbare), Complete is
// alleen true als alle listings gelukt zijn. BaseURL is de gebruikte root-URL.
type StaticHarvest struct {
	BaseURL    string
	Files      []StaticSchemaFile
	ListedURLs []string
	Complete   bool
}

type SchemaPost struct {
	Id              string  `json:"id,omitempty"`
	SchemaUrl       string  `json:"schemaUrl" binding:"required_without=SchemaBody,omitempty,url"`
	SchemaBody      string  `json:"schemaBody,omitempty" binding:"required_without=SchemaUrl"`
	OrganisationUri string  `json:"organisationUri" binding:"required,url"`
	Contact         Contact `json:"contact"`
}

type UpdateSchemaInput struct {
	Id              string  `path:"id"`
	SchemaUrl       string  `json:"schemaUrl" binding:"required_without=SchemaBody,omitempty,url"`
	SchemaBody      string  `json:"schemaBody,omitempty"`
	OrganisationUri string  `json:"organisationUri" binding:"required,url"`
	Contact         Contact `json:"contact"`
}

type ListSchemasSearchParams struct {
	Page         int     `query:"page" validate:"omitempty,min=1"`
	PerPage      int     `query:"perPage" validate:"omitempty,min=1,max=100"`
	Organisation *string `query:"organisation"`
	Query        string  `query:"q" binding:"required"`
	BaseURL      string
}

type ListSchemasParams struct {
	Page         int      `query:"page" validate:"omitempty,min=1"`
	PerPage      int      `query:"perPage" validate:"omitempty,min=1,max=100"`
	Organisation *string  `query:"organisation"`
	Query        string   `query:"q"`
	Dialect      []string `query:"dialect"`
	RootType     []string `query:"rootType"`
	BaseURL      string
}

func (p *ListSchemasParams) SchemaFilters() *SchemaFiltersParams {
	if p == nil {
		return &SchemaFiltersParams{}
	}
	return &SchemaFiltersParams{
		Query:    p.Query,
		Dialect:  append([]string(nil), p.Dialect...),
		RootType: append([]string(nil), p.RootType...),
	}
}

type SchemaParams struct {
	Id string `path:"id"`
}

// Pagination, FilterOption, FilterGroup en FilterCount worden gedeeld via
// don-register-common, net als in het API-register.
type Pagination = commonpagination.Pagination

type FilterOption = commonfilters.FilterOption

type FilterGroup = commonfilters.FilterGroup

type FilterCount = commonfilters.FilterCount

type SchemaFilterCounts struct {
	Dialect      []FilterCount
	RootType     []FilterCount
	Organisation []FilterCount
}

type SchemaFiltersParams struct {
	Organisation *string  `query:"organisation"`
	Query        string   `query:"q"`
	Dialect      []string `query:"dialect"`
	RootType     []string `query:"rootType"`
}
