// Package gormadapter lets projects opt into GORM on the application's SQL pool.
package gormadapter

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open wraps db without opening or owning another pool. A nil writer uses the standard logger.
func Open(db *sqlx.DB, slowThreshold time.Duration, writer logger.Writer) (*gorm.DB, error) {
	if db == nil || db.DB == nil {
		return nil, errors.New("GORM requires an enabled SQL connection pool")
	}
	var dialect gorm.Dialector
	switch db.DriverName() {
	case "mysql":
		// The application uses MySQL 8, so initialization needs no version query.
		dialect = mysql.New(mysql.Config{Conn: db.DB, SkipInitializeWithVersion: true})
	case "pgx", "postgres":
		dialect = postgres.New(postgres.Config{Conn: db.DB})
	default:
		return nil, fmt.Errorf("GORM adapter does not support driver %q", db.DriverName())
	}
	if writer == nil {
		writer = log.Default()
	}
	return gorm.Open(dialect, &gorm.Config{
		DisableAutomaticPing: true,
		Logger: logger.New(writer, logger.Config{
			LogLevel:                  logger.Warn,
			SlowThreshold:             slowThreshold,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
	})
}
