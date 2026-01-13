package discord

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sniper/config"
	"strings"
)

// Structures pour l'API Discord
type DiscordMessage struct {
	Embeds     []DiscordEmbed     `json:"embeds"`
	Components []DiscordComponent `json:"components"`
}
type DiscordEmbed struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	URL         string        `json:"url"`
	Color       int           `json:"color"`
	Image       DiscordImage  `json:"image"`
	Footer      DiscordFooter `json:"footer"`
}
type DiscordImage struct {
	URL string `json:"url"`
}
type DiscordFooter struct {
	Text string `json:"text"`
}
type DiscordComponent struct {
	Type       int             `json:"type"`
	Components []DiscordButton `json:"components"`
}
type DiscordButton struct {
	Type  int    `json:"type"`
	Label string `json:"label"`
	Style int    `json:"style"`
	URL   string `json:"url"`
}

// Envoyer : Gère le routage et l'envoi
func Envoyer(cote, prix int, titre, desc, lien, img string, couleur int, source string) {
	webhook := config.WB_LUXE // Par défaut > 50k
	marge := 0.0

	if cote > 0 {
		marge = (float64(cote-prix) / float64(cote)) * 100
	}

	// 1. ROUTAGE INTELLIGENT
	if marge > 20 { // Priorité absolue aux Super Affaires
		webhook = config.WB_ADMIN
	} else {
		// Sinon, tri par budget
		if prix <= 10000 {
			webhook = config.WB_0_10K
		} else if prix <= 20000 {
			webhook = config.WB_10_20K
		} else if prix <= 30000 {
			webhook = config.WB_20_30K
		} else if prix <= 50000 {
			webhook = config.WB_30_50K
		}
	}

	// Sécurité : Si le webhook est vide ou non configuré
	if webhook == "" || strings.Contains(webhook, "WEBHOOK_") {
		return
	}

	// 2. NETTOYAGE SOURCE (Pour le Footer)
	// On garde juste "Argus Expert" au lieu de "Argus Expert B (Stable)..."
	sourceClean := strings.Split(source, "(")[0]
	sourceClean = strings.TrimSpace(sourceClean)

	// 3. CONSTRUCTION DU MESSAGE
	payload := DiscordMessage{
		Embeds: []DiscordEmbed{{
			Title:       titre,
			Description: desc, // La description propre faite dans le scraper
			URL:         lien,
			Color:       couleur,
			Image:       DiscordImage{URL: img},
			//Footer:      DiscordFooter{Text: "Source: " + sourceClean}, // Source discrète en bas
		}},
		Components: []DiscordComponent{{
			Type: 1,
			Components: []DiscordButton{{
				Type: 2, Label: "Voir l'annonce", Style: 5, URL: lien,
			}},
		}},
	}

	jsonBody, _ := json.Marshal(payload)
	http.Post(webhook, "application/json", bytes.NewBuffer(jsonBody))
}

// Fonction pour le flux gratuit différé
func EnvoyerGratuit(webhook, titre, desc, lien, img string) {
	payload := DiscordMessage{
		Embeds:     []DiscordEmbed{{Title: titre, Description: desc, URL: lien, Color: 9807270, Image: DiscordImage{URL: img}, Footer: DiscordFooter{Text: "Passez VIP"}}},
		Components: []DiscordComponent{{Type: 1, Components: []DiscordButton{{Type: 2, Label: "Voir", Style: 5, URL: lien}}}},
	}
	jsonBody, _ := json.Marshal(payload)
	http.Post(webhook, "application/json", bytes.NewBuffer(jsonBody))
}
