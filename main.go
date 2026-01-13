package main

import (
	"fmt"
	"time"

	"sniper/config"
	"sniper/database"
	"sniper/discord"
	"sniper/scraper"
)

func main() {
	fmt.Println("🏗️  Démarrage du Sniper Auto - MULTI SOURCES")

	database.InitDB()
	defer database.DB.Close()

	// 1. Flux Gratuit (Background)
	go gestionFluxGratuit()

	// 2. Scraper LEBONCOIN (Background)
	// On le lance avec "go" pour qu'il ne bloque pas le programme
	go scraper.Lancer()
	go scraper.LancerAutoScout()
	// 3. Scraper LA CENTRALE (Premier plan)
	// On lance celui-ci en dernier, il va garder le programme ouvert
	// (Ou on peut lancer les deux en 'go' et mettre un select{} à la fin)
	//scraper.LancerLC()
	select {}
}

// --- FLUX GRATUIT ---
func gestionFluxGratuit() {
	fmt.Println("⏳ Service 'Flux Gratuit' activé.")
	for {
		time.Sleep(60 * time.Second)
		rows, err := database.DB.Query(`
			SELECT id, site, titre, prix, annee, km, carburant, ville, url, image 
			FROM annonces 
			WHERE envoye_gratuit = 0 
			AND date_creation <= datetime('now', '-15 minutes')
		`)
		if err != nil {
			continue
		}

		for rows.Next() {
			var id, site, titre, ville, url, image, carburant string
			var prix, annee, km int
			rows.Scan(&id, &site, &titre, &prix, &annee, &km, &carburant, &ville, &url, &image)

			prefix := "🟠" // Leboncoin
			if site == "LaCentrale" {
				prefix = "🔵"
			}

			desc := fmt.Sprintf("**Prix:** %d €\n**Année:** %d\n**Km:** %d km\n⛽ %s\n📍 %s\nSource: %s", prix, annee, km, carburant, ville, site)
			discord.EnvoyerGratuit(config.WB_GRATUIT, prefix+" "+titre, desc, url, image)

			// On update en fonction de l'ID ET du SITE
			database.DB.Exec("UPDATE annonces SET envoye_gratuit = 1 WHERE id = ? AND site = ?", id, site)
			time.Sleep(2 * time.Second)
		}
		rows.Close()
	}
}
