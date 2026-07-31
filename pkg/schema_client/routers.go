package schema_client

import (
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/handler"
	commonrouter "github.com/developer-overheid-nl/don-register-common/router"
	"github.com/gin-gonic/gin"
	"github.com/loopfz/gadgeto/tonic"
	"github.com/wI2L/fizz"
	"github.com/wI2L/fizz/openapi"
)

var (
	apiVersionHeader = fizz.Header(
		"API-Version",
		"De API-versie van de response",
		"",
	)
)

func NewRouter(apiVersion string, controller *handler.SchemaController) *fizz.Fizz {
	//gin.SetMode(gin.ReleaseMode)
	g := commonrouter.NewEngine(apiVersion, commonrouter.CORSOptions{
		AllowHeaders:  []string{"Origin", "Content-Length", "Content-Type", "Authorization", "API-Version", "X-Api-Key"},
		ExposeHeaders: []string{"API-Version", "Link", "Total-Count", "Total-Pages", "Per-Page", "Current-Page"},
	})
	commonrouter.InstallProblemHandlers(g, apiVersion)
	f := fizz.NewFromEngine(g)

	root := f.Group("/v1", "Schema v1", "Schema Register V1 routes")

	root.GET("/schemas",
		[]fizz.OperationOption{
			fizz.ID("listSchemas"),
			fizz.Summary("List schemas"),
			fizz.Description("Geeft een lijst terug met JSON Schemas die in het register zijn opgenomen. Ondersteunt dezelfde filterquery's als het filterendpoint en combineert deze met de optionele zoekterm q."),
			fizz.Security(&openapi.SecurityRequirement{
				"clientCredentials": {},
			}),
			apiVersionHeader,
		},
		tonic.Handler(controller.ListSchemas, 200),
	)

	root.GET("/schemas/filters",
		[]fizz.OperationOption{
			fizz.ID("listSchemaFilters"),
			fizz.Summary("Filter opties ophalen"),
			fizz.Description("Geeft alle beschikbare filteropties terug met counts. Counts zijn berekend op basis van de meegegeven actieve filters en de optionele zoekterm q."),
			fizz.Security(&openapi.SecurityRequirement{
				"clientCredentials": {},
			}),
			apiVersionHeader,
		},
		tonic.Handler(controller.ListSchemaFilters, 200),
	)

	root.GET("/schemas/:id",
		[]fizz.OperationOption{
			fizz.ID("getSchemaById"),
			fizz.Summary("Get schema by id"),
			fizz.Description("Geeft één JSON Schema terug op basis van het id, inclusief de opgeslagen schema-inhoud."),
			fizz.Security(&openapi.SecurityRequirement{
				"apiKey":            {},
				"clientCredentials": {"schemas:read"},
			}),
			apiVersionHeader,
		},
		tonic.Handler(controller.RetrieveSchema, 200),
	)

	root.PUT("/schemas/:id",
		[]fizz.OperationOption{
			fizz.ID("updateSchema"),
			fizz.Summary("Specifiek schema updaten"),
			fizz.Description("Haalt het JSON Schema opnieuw op via schemaUrl (of gebruikt schemaBody) en werkt het geregistreerde schema bij."),
			fizz.Security(&openapi.SecurityRequirement{
				"clientCredentials": {"schemas:write"},
			}),
			apiVersionHeader,
		},
		tonic.Handler(controller.UpdateSchema, 200),
	)

	root.POST("/schemas",
		[]fizz.OperationOption{
			fizz.ID("createSchema"),
			fizz.Summary("Create schema"),
			fizz.Description("Registreer een nieuw JSON Schema in het register op basis van een schemaUrl of schemaBody. Het schema wordt opgehaald, gevalideerd en met metadata opgeslagen."),
			fizz.Security(&openapi.SecurityRequirement{
				"clientCredentials": {"schemas:write"},
			}),
			apiVersionHeader,
		},
		tonic.Handler(controller.CreateSchema, 201),
	)

	root.GET("/organisations",
		[]fizz.OperationOption{
			fizz.ID("listOrganisations"),
			fizz.Summary("Alle organisaties ophalen"),
			fizz.Description("Alle organisaties ophalen"),
			fizz.Security(&openapi.SecurityRequirement{
				"apiKey":            {},
				"clientCredentials": {"organisations:read"},
			}),
			apiVersionHeader,
		},
		tonic.Handler(controller.ListOrganisations, 200),
	)

	root.POST("/organisations",
		[]fizz.OperationOption{
			fizz.ID("createOrganisation"),
			fizz.Summary("Organisatie aanmaken"),
			fizz.Description("Maak een nieuwe organisatie aan."),
			fizz.Security(&openapi.SecurityRequirement{
				"clientCredentials": {"organisations:write"},
			}),
			apiVersionHeader,
		},
		tonic.Handler(controller.CreateOrganisation, 201),
	)

	// Raw schema-inhoud; geregistreerd op de onderliggende engine zodat de
	// handler zelf de content type (application/schema+json) kan bepalen.
	g.GET("/v1/schemas/:id/schema.json", controller.RetrieveSchemaContent)

	// OpenAPI documentatie
	g.StaticFile("/v1/openapi.json", "./api/openapi.json")

	return f
}

func APIVersionMiddleware(version string) gin.HandlerFunc {
	return commonrouter.APIVersionMiddleware(version)
}
