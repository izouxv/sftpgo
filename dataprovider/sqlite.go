package dataprovider

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/drakkan/sftpgo/logger"
	"github.com/drakkan/sftpgo/utils"
)

const (
	sqliteUsersTableSQL = `CREATE TABLE "{{users}}" ("id" integer NOT NULL PRIMARY KEY AUTOINCREMENT, "username" varchar(255)
NOT NULL UNIQUE, "password" varchar(255) NULL, "public_keys" text NULL, "home_dir" varchar(255) NOT NULL, "uid" integer NOT NULL,
"gid" integer NOT NULL, "max_sessions" integer NOT NULL, "quota_size" bigint NOT NULL, "quota_files" integer NOT NULL,
"permissions" text NOT NULL, "used_quota_size" bigint NOT NULL, "used_quota_files" integer NOT NULL,
"last_quota_update" bigint NOT NULL, "upload_bandwidth" integer NOT NULL, "download_bandwidth" integer NOT NULL,
"expiration_date" bigint NOT NULL, "last_login" bigint NOT NULL, "status" integer NOT NULL, "filters" text NULL,
"filesystem" text NULL);`
	sqliteSchemaTableSQL = `CREATE TABLE "schema_version" ("id" integer NOT NULL PRIMARY KEY AUTOINCREMENT, "version" integer NOT NULL);`
	sqliteUsersV2SQL     = `ALTER TABLE "{{users}}" ADD COLUMN "virtual_folders" text NULL;`
	sqliteUsersV3SQL     = `CREATE TABLE "new__users" ("id" integer NOT NULL PRIMARY KEY AUTOINCREMENT, "username" varchar(255) NOT NULL UNIQUE,
	"password" text NULL, "public_keys" text NULL, "home_dir" varchar(255) NOT NULL, "uid" integer NOT NULL,
"gid" integer NOT NULL, "max_sessions" integer NOT NULL, "quota_size" bigint NOT NULL, "quota_files" integer NOT NULL,
"permissions" text NOT NULL, "used_quota_size" bigint NOT NULL, "used_quota_files" integer NOT NULL, "last_quota_update" bigint NOT NULL,
"upload_bandwidth" integer NOT NULL, "download_bandwidth" integer NOT NULL, "expiration_date" bigint NOT NULL, "last_login" bigint NOT NULL,
"status" integer NOT NULL, "filters" text NULL, "filesystem" text NULL, "virtual_folders" text NULL);
INSERT INTO "new__users" ("id", "username", "public_keys", "home_dir", "uid", "gid", "max_sessions", "quota_size", "quota_files",
"permissions", "used_quota_size", "used_quota_files", "last_quota_update", "upload_bandwidth", "download_bandwidth", "expiration_date",
"last_login", "status", "filters", "filesystem", "virtual_folders", "password") SELECT "id", "username", "public_keys", "home_dir",
"uid", "gid", "max_sessions", "quota_size", "quota_files", "permissions", "used_quota_size", "used_quota_files", "last_quota_update",
"upload_bandwidth", "download_bandwidth", "expiration_date", "last_login", "status", "filters", "filesystem", "virtual_folders",
"password" FROM "{{users}}";
DROP TABLE "{{users}}";
ALTER TABLE "new__users" RENAME TO "{{users}}";`
)

// SQLiteProvider auth provider for SQLite database
type SQLiteProvider struct {
	dbHandle *sql.DB
}

func initializeSQLiteProvider(basePath string) error {
	var err error
	var connectionString string
	logSender = fmt.Sprintf("dataprovider_%v", SQLiteDataProviderName)
	if len(config.ConnectionString) == 0 {
		dbPath := config.Name
		if !utils.IsFileInputValid(dbPath) {
			return fmt.Errorf("Invalid database path: %#v", dbPath)
		}
		if !filepath.IsAbs(dbPath) {
			dbPath = filepath.Join(basePath, dbPath)
		}
		connectionString = fmt.Sprintf("file:%v?cache=shared", dbPath)
	} else {
		connectionString = config.ConnectionString
	}
	dbHandle, err := sql.Open("sqlite3", connectionString)
	if err == nil {
		providerLog(logger.LevelDebug, "sqlite database handle created, connection string: %#v", connectionString)
		dbHandle.SetMaxOpenConns(1)
		provider = SQLiteProvider{dbHandle: dbHandle}
	} else {
		providerLog(logger.LevelWarn, "error creating sqlite database handler, connection string: %#v, error: %v",
			connectionString, err)
	}
	return err
}

func (p SQLiteProvider) checkAvailability() error {
	return sqlCommonCheckAvailability(p.dbHandle)
}

func (p SQLiteProvider) validateUserAndPass(username string, password string) (User, error) {
	return sqlCommonValidateUserAndPass(username, password, p.dbHandle)
}

func (p SQLiteProvider) validateUserAndPubKey(username string, publicKey []byte) (User, string, error) {
	return sqlCommonValidateUserAndPubKey(username, publicKey, p.dbHandle)
}

func (p SQLiteProvider) getUserByID(ID int64) (User, error) {
	return sqlCommonGetUserByID(ID, p.dbHandle)
}

func (p SQLiteProvider) updateQuota(username string, filesAdd int, sizeAdd int64, reset bool) error {
	return sqlCommonUpdateQuota(username, filesAdd, sizeAdd, reset, p.dbHandle)
}

func (p SQLiteProvider) updateLastLogin(username string) error {
	return sqlCommonUpdateLastLogin(username, p.dbHandle)
}

func (p SQLiteProvider) getUsedQuota(username string) (int, int64, error) {
	return sqlCommonGetUsedQuota(username, p.dbHandle)
}

func (p SQLiteProvider) userExists(username string) (User, error) {
	return sqlCommonCheckUserExists(username, p.dbHandle)
}

func (p SQLiteProvider) addUser(user User) error {
	return sqlCommonAddUser(user, p.dbHandle)
}

func (p SQLiteProvider) updateUser(user User) error {
	return sqlCommonUpdateUser(user, p.dbHandle)
}

func (p SQLiteProvider) deleteUser(user User) error {
	return sqlCommonDeleteUser(user, p.dbHandle)
}

func (p SQLiteProvider) dumpUsers() ([]User, error) {
	return sqlCommonDumpUsers(p.dbHandle)
}

func (p SQLiteProvider) getUsers(limit int, offset int, order string, username string) ([]User, error) {
	return sqlCommonGetUsers(limit, offset, order, username, p.dbHandle)
}

func (p SQLiteProvider) close() error {
	return p.dbHandle.Close()
}

func (p SQLiteProvider) reloadConfig() error {
	return nil
}

// initializeDatabase creates the initial database structure
func (p SQLiteProvider) initializeDatabase() error {
	sqlUsers := strings.Replace(sqliteUsersTableSQL, "{{users}}", config.UsersTable, 1)
	sql := sqlUsers + " " + sqliteSchemaTableSQL + " " + initialDBVersionSQL
	_, err := p.dbHandle.Exec(sql)
	return err
}

func (p SQLiteProvider) migrateDatabase() error {
	dbVersion, err := sqlCommonGetDatabaseVersion(p.dbHandle)
	if err != nil {
		return err
	}
	if dbVersion.Version == sqlDatabaseVersion {
		providerLog(logger.LevelDebug, "sql database is updated, current version: %v", dbVersion.Version)
		return nil
	}
	switch dbVersion.Version {
	case 1:
		err = updateSQLiteDatabaseFrom1To2(p.dbHandle)
		if err != nil {
			return err
		}
		return updateSQLiteDatabaseFrom2To3(p.dbHandle)
	case 2:
		return updateSQLiteDatabaseFrom2To3(p.dbHandle)
	default:
		return fmt.Errorf("Database version not handled: %v", dbVersion.Version)
	}
}

func updateSQLiteDatabaseFrom1To2(dbHandle *sql.DB) error {
	providerLog(logger.LevelInfo, "updating database version: 1 -> 2")
	sql := strings.Replace(sqliteUsersV2SQL, "{{users}}", config.UsersTable, 1)
	_, err := dbHandle.Exec(sql)
	if err != nil {
		return err
	}
	return sqlCommonUpdateDatabaseVersion(dbHandle, 2)
}

func updateSQLiteDatabaseFrom2To3(dbHandle *sql.DB) error {
	providerLog(logger.LevelInfo, "updating database version: 2 -> 3")
	sql := strings.ReplaceAll(sqliteUsersV3SQL, "{{users}}", config.UsersTable)
	_, err := dbHandle.Exec(sql)
	if err != nil {
		return err
	}
	return sqlCommonUpdateDatabaseVersion(dbHandle, 3)
}
