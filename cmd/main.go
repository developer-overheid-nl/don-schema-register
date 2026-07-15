package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/handler"
	util "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/helpers/util"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	api "github.com/developer-overheid-nl/don-schema-register/pkg/schema_client"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/database"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/jobs"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/schemas"
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/services"
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

	jobs.NewSchemaRefreshJob(schemasService, context.Background())

	// Start server
	router := api.NewRouter(version, controller)

	log.Println("Server is running on port 1337")
	log.Fatal(http.ListenAndServe(":1337", router))
}
