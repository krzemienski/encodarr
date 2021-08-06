package sqlite

import (
	"database/sql"
	"embed"
	"errors"
	"io"
	"os"

	_ "modernc.org/sqlite"

	"github.com/BrenekH/encodarr/controller"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/sqlite"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

//go:embed migrations
var migrations embed.FS

const targetMigrationVersion uint = 2

type SQLiteDatabase struct {
	Client *sql.DB
}

func NewSQLiteDatabase(configDir string, logger controller.Logger) (SQLiteDatabase, error) {
	dbFile := configDir + "/data.db"
	dbBckpFile := configDir + "/data.db.backup"

	client, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return SQLiteDatabase{Client: client}, err
	}

	// Set max connections to 1 to prevent "database is locked" errors
	client.SetMaxOpenConns(1)

	dbBackup, err := os.Create(dbBckpFile)
	if err != nil {
		return SQLiteDatabase{Client: client}, err
	}

	err = gotoDBVer(dbFile, targetMigrationVersion, configDir, dbBackup, logger)

	return SQLiteDatabase{Client: client}, err
}

// gotoDBVer uses github.com/golang-migrate/migrate to move the db version up or down to the passed target version.
func gotoDBVer(dbFile string, targetVersion uint, configDir string, backupWriter io.Writer, logger controller.Logger) error {
	// Instead of directly using the embedded files, write them out to {configDir}/migrations. This allows the files for downgrading the
	// database to be present even when the executable doesn't contain them.
	fsMigrationsDir := configDir + "/migrations"

	if err := os.MkdirAll(fsMigrationsDir, 0777); err != nil {
		return err
	}

	dirEntries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}

	var copyErred bool
	for _, v := range dirEntries {
		f, err := os.Create(fsMigrationsDir + "/" + v.Name())
		if err != nil {
			logger.Error("%v", err)
			copyErred = true
			continue
		}

		embeddedFile, err := migrations.Open("migrations/" + v.Name())
		if err != nil {
			logger.Error("%v", err)
			copyErred = true
			f.Close()
			continue
		}

		if _, err := io.Copy(f, embeddedFile); err != nil {
			logger.Error("%v", err)
			copyErred = true
			// Don't continue right here so that the files are closed before looping again
		}

		f.Close()
		embeddedFile.Close()
	}
	if copyErred {
		return errors.New("error(s) while copying migrations, check logs for more details")
	}

	mig, err := migrate.New("file://"+configDir+"/migrations", "sqlite://"+dbFile)
	if err != nil {
		return err
	}
	defer mig.Close()

	currentVer, _, err := mig.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			// DB is likely before golang-migrate was introduced. Upgrade to new version
			logger.Warn("Database does not have a schema version. Attempting to migrate up.")
			err = backupFile(dbFile, backupWriter, logger)
			if err != nil {
				return err
			}

			return mig.Migrate(targetVersion)
		}
		return err
	}

	if currentVer == targetVersion {
		return nil
	}

	err = backupFile(dbFile, backupWriter, logger)
	if err != nil {
		return err
	}

	logger.Info("Migrating database to schema version %v.", targetVersion)
	return mig.Migrate(targetVersion)
}

func backupFile(from string, to io.Writer, logger controller.Logger) error {
	fromReader, err := os.Open(from)
	if err != nil {
		return err
	}

	logger.Info("Backing up database.")
	_, err = io.Copy(to, fromReader)
	return err
}