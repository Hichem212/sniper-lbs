package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() {
	var err error
	DB, err = sql.Open("sqlite3", "./voitures.db?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		panic(err)
	}
	DB.Exec("PRAGMA journal_mode=WAL;")

	// Ajout colonne SITE
	DB.Exec(`CREATE TABLE IF NOT EXISTS annonces (
		id TEXT, 
		site TEXT, 
		titre TEXT, prix INTEGER, annee INTEGER, km INTEGER, carburant TEXT, ville TEXT,
		estimation_ia INTEGER, url TEXT, image TEXT, envoye_gratuit INTEGER DEFAULT 0,
		date_creation DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id, site)
	);`)
	fmt.Println("📂 Base de données chargée (Multi-Sources).")
}

func InsertAnnonce(id, site, titre string, prix, annee, km int, carb, ville string, cote int, url, img string) {
	// On insère en précisant le site
	DB.Exec("INSERT INTO annonces (id, site, titre, prix, annee, km, carburant, ville, estimation_ia, url, image, envoye_gratuit) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)", id, site, titre, prix, annee, km, carb, ville, cote, url, img)
}

func Exists(id, site string) bool {
	var exists int
	// On vérifie l'ID ET le SITE
	err := DB.QueryRow("SELECT 1 FROM annonces WHERE id = ? AND site = ?", id, site).Scan(&exists)
	return err == nil
}
