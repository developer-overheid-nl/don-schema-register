package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/handler"
	util "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/util"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	api "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/database"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/jobs"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/schemas"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/services"
	"github.com/developer-overheid-nl/don-schema-register/seed"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file ", err)
	}

	version, err := util.LoadOASVersion("./api/openapi.json")
	if err != nil {
		log.Fatalf("failed to load OAS version: %v", err)
	}
	host := os.Getenv("DB_HOSTNAME")
	user := os.Getenv("DB_USERNAME")
	pass := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_DBNAME")
	schema := os.Getenv("DB_SCHEMA")

	u := &url.URL{
		Scheme: "postgres",
		Host:   host + ":5432",
		Path:   dbname,
	}
	u.User = url.UserPassword(user, pass)

	q := u.Query()
	// q.Set("sslmode", "require")
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	dbcon := u.String()
	db, err := database.Connect(dbcon)
	if err != nil {
		log.Fatalf("Geen databaseverbinding: %v", err)
	}
	repo := schemas.NewSchemasRepository(db)
	schemasService := services.NewSchemaService(repo)
	controller := handler.NewSchemaController(schemasService)
	if _, err := schemasService.CreateOrganisation(context.Background(), &models.Organisation{Uri: "https://www.gpp-woo.nl", Label: "GPP-Woo"}); err != nil {
		fmt.Printf("[GPP-Woo-import] create org warning: %v\n", err)
	}
	if _, err := schemasService.CreateOrganisation(context.Background(), &models.Organisation{Uri: "https://www.geonovum.nl", Label: "Stichting Geonovum"}); err != nil {
		fmt.Printf("[Geonovum-import] create org warning: %v\n", err)
	}
	if _, err := schemasService.CreateOrganisation(context.Background(), &models.Organisation{Uri: "https://www.ictu.nl", Label: "ICTU"}); err != nil {
		fmt.Printf("[ICTU-import] create org warning: %v\n", err)
	}
	if _, err := schemasService.CreateOrganisation(context.Background(), &models.Organisation{Uri: "https://vng.nl", Label: "Vereniging van Nederlandse Gemeenten"}); err != nil {
		fmt.Printf("[VNG-import] create org warning: %v\n", err)
	}
	if _, err := schemasService.CreateOrganisation(context.Background(), &models.Organisation{Uri: "https://developer.overheid.nl/", Label: "Developer overheid"}); err != nil {
		fmt.Printf("[Developer-overheid-import] create org warning: %v\n", err)
	}
	if _, err := schemasService.CreateOrganisation(context.Background(), &models.Organisation{Uri: "https://www.logius.nl", Label: "Logius"}); err != nil {
		fmt.Printf("[Logius-import] create org warning: %v\n", err)
	}
	if err := seed.SeedSchemas(context.Background(), repo); err != nil {
		log.Fatalf("[seed] seeding schemas failed: %v", err)
	}
	if err := schemasService.PublishAllSchemasToTypesense(context.Background()); err != nil {
		log.Fatalf("[typesense-sync] bulk publish failed: %v", err)
	}
	jobs.NewSchemaRefreshJob(schemasService, context.Background())

	// Start server
	router := api.NewRouter(version, controller)

	log.Println("Server is running on port 1337")
	log.Fatal(http.ListenAndServe(":1337", router))
}
