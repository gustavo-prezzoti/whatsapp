package config

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

type DatabaseConfig struct {
	Server   string
	Database string
	User     string
	Password string
}

func NewDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Server:   "ligchat.cmj46oa0g26n.us-east-1.rds.amazonaws.com",
		Database: "ligchat",
		User:     "ligchat",
		Password: "Cap0199**",
	}
}

func (c *DatabaseConfig) GetDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true&multiStatements=true&loc=America%%2FManaus&time_zone='-04:00'",
		c.User, c.Password, c.Server, c.Database)
}

func ConnectDatabase() (*sql.DB, error) {
	config := NewDatabaseConfig()
	db, err := sql.Open("mysql", config.GetDSN())
	if err != nil {
		return nil, fmt.Errorf("error opening database: %v", err)
	}

	// Configurar o pool de conexões
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	// Testar a conexão
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("error connecting to the database: %v", err)
	}

	// Configurar fuso horário explicitamente para todas as conexões
	_, err = db.Exec("SET time_zone = 'America/Manaus'")
	if err != nil {
		return nil, fmt.Errorf("error setting timezone: %v", err)
	}

	return db, nil
}
