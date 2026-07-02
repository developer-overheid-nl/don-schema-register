package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	problem "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/problem"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/util"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/services"
	"github.com/gin-gonic/gin"
)

// SchemaController binds HTTP requests to the SchemaController
type SchemaController struct {
	Service *services.SchemaService
}

// NewSchemaController creates a new controller
func NewSchemaController(s *services.SchemaService) *SchemaController {
	return &SchemaController{Service: s}
}

// ListSchemas handles GET /schemas
func (c *SchemaController) ListSchemas(ctx *gin.Context, p *models.ListSchemasParams) ([]models.SchemaSummary, error) {
	p.Page, p.PerPage = normalizePagination(p.Page, p.PerPage)
	p.BaseURL = ctx.FullPath()
	dtos, pagination, err := c.Service.ListSchemas(ctx.Request.Context(), p)
	if err != nil {
		return nil, err
	}
	util.SetPaginationHeaders(ctx.Request, ctx.Header, pagination)

	return dtos, nil
}

// RetrieveSchema handles GET /schemas/:id
func (c *SchemaController) RetrieveSchema(ctx *gin.Context, params *models.SchemaParams) (*models.SchemaDetail, error) {
	schema, err := c.Service.RetrieveSchema(ctx.Request.Context(), params.Id)
	if err != nil {
		return nil, err
	}
	if schema == nil {
		return nil, problem.NewNotFound("Resource does not exist")
	}
	return schema, nil
}

// RetrieveSchemaContent handles GET /schemas/:id/schema.json and serves the
// raw stored JSON Schema with the application/schema+json content type. This
// is registered as a plain gin handler (not via tonic) so it can control the
// response body and content type directly.
func (c *SchemaController) RetrieveSchemaContent(ctx *gin.Context) {
	schema, err := c.Service.RetrieveSchemaModel(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		var p problem.ProblemJSON
		if errors.As(err, &p) {
			ctx.JSON(p.Status, p)
			return
		}
		ctx.JSON(http.StatusInternalServerError, problem.NewInternalServerError(err.Error()))
		return
	}
	if schema == nil || len(schema.Content) == 0 {
		notFound := problem.NewNotFound("Resource does not exist")
		ctx.JSON(notFound.Status, notFound)
		return
	}
	payload, err := json.MarshalIndent(schema.Content, "", "  ")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, problem.NewInternalServerError("kan schema niet serialiseren: "+err.Error()))
		return
	}
	ctx.Data(http.StatusOK, "application/schema+json", payload)
}

// CreateSchema handles POST /schemas
func (c *SchemaController) CreateSchema(ctx *gin.Context, body *models.SchemaPost) (*models.SchemaSummary, error) {
	created, err := c.Service.CreateSchemaFromInput(*body)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// UpdateSchema handles PUT /schemas/:id
func (c *SchemaController) UpdateSchema(ctx *gin.Context, body *models.UpdateSchemaInput) (*models.SchemaSummary, error) {
	updated, err := c.Service.UpdateSchemaFromInput(ctx.Request.Context(), body)
	if errors.Is(err, services.ErrNeedsPost) {
		return nil, problem.NewNotFound(fmt.Sprintf("'%s' moet als nieuw schema geregistreerd worden via POST", body.SchemaUrl))
	}
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// ListSchemaFilters handles GET /schemas/filters
func (c *SchemaController) ListSchemaFilters(ctx *gin.Context, p *models.SchemaFiltersParams) ([]models.FilterGroup, error) {
	return c.Service.GetSchemaFilters(ctx.Request.Context(), p)
}

// ListOrganisations handles GET /organisations
func (c *SchemaController) ListOrganisations(ctx *gin.Context, p *models.ListOrganisationsParams) ([]models.OrganisationSummary, error) {
	p.Page, p.PerPage = normalizePagination(p.Page, p.PerPage)
	p.PerPage = 100
	p.BaseURL = ctx.FullPath()
	orgs, pagination, err := c.Service.ListOrganisations(ctx.Request.Context(), p)
	if err != nil {
		return nil, err
	}
	util.SetPaginationHeaders(ctx.Request, ctx.Header, pagination)

	return orgs, nil
}

// CreateOrganisation handles POST /organisations
func (c *SchemaController) CreateOrganisation(ctx *gin.Context, body *models.Organisation) (*models.Organisation, error) {
	created, err := c.Service.CreateOrganisation(ctx.Request.Context(), body)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func normalizePagination(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}
