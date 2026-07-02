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
	Id             string               `json:"id" gorm:"column:id;primaryKey"`
	SchemaUrl      string               `json:"schemaUrl,omitempty" gorm:"column:schema_url"`
	Title          string               `json:"title,omitempty" gorm:"column:title"`
	Description    string               `json:"description,omitempty" gorm:"column:description"`
	Dialect        string               `json:"dialect,omitempty" gorm:"column:dialect"`
	RootType       string               `json:"rootType,omitempty" gorm:"column:root_type"`
	Collection     string               `json:"collection,omitempty" gorm:"column:collection"`
	Contact        Contact              `json:"contact" gorm:"-"`
	Organisation   *OrganisationSummary `json:"organisation,omitempty" gorm:"foreignKey:OrganisationID;references:Uri"`
	OrganisationID *string              `json:"-" gorm:"column:organisation_id"`
	CreatedAt      time.Time            `json:"createdAt" gorm:"column:created_at"`
	LastCrawledAt  time.Time            `json:"lastCrawledAt" gorm:"column:last_crawled_at"`
}

type SchemaDetail struct {
	SchemaSummary
	Content map[string]any `json:"content,omitempty"`
}

type Schema struct {
	Id             string         `json:"id" gorm:"column:id;primaryKey"`
	SchemaUrl      string         `json:"schemaUrl,omitempty" gorm:"column:schema_url"`
	Title          string         `json:"title" gorm:"column:title"`
	Description    string         `json:"description,omitempty" gorm:"column:description"`
	Dialect        string         `json:"dialect,omitempty" gorm:"column:dialect"`
	RootType       string         `json:"rootType,omitempty" gorm:"column:root_type"`
	Collection     string         `json:"collection,omitempty" gorm:"column:collection"`
	ContactName    string         `json:"contact_name,omitempty" gorm:"column:contact_name"`
	ContactUrl     string         `json:"contact_url,omitempty" gorm:"column:contact_url"`
	ContactEmail   string         `json:"contact_email,omitempty" gorm:"column:contact_email"`
	Organisation   *Organisation  `json:"-" gorm:"foreignKey:OrganisationID;references:Uri"`
	OrganisationID *string        `json:"-" gorm:"column:organisation_id"`
	Content        map[string]any `json:"content,omitempty" gorm:"column:content;serializer:json"`
	Hash           string         `json:"-" gorm:"column:schema_hash"`
	CreatedAt      time.Time      `json:"createdAt" gorm:"column:created_at"`
	LastCrawledAt  time.Time      `json:"lastCrawledAt" gorm:"column:last_crawled_at"`
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
		Organisation: p.Organisation,
		Query:        p.Query,
		Dialect:      append([]string(nil), p.Dialect...),
		RootType:     append([]string(nil), p.RootType...),
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
