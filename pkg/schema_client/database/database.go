package database

import (
	"github.com/developer-overheid-nl/don-schema-register/pkg/schema_client/models"
	commondatabase "github.com/developer-overheid-nl/don-register-common/database"
	_ "github.com/lib/pq"
	"gorm.io/gorm"
)

// Connect connects to the database and performs migrations.
func Connect(connStr string) (*gorm.DB, error) {
	return commondatabase.ConnectPostgres(connStr,
		&models.Organisation{},
		&models.Schema{},
	)
}
