package discord

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sniper/config" // On importe la config
	"strings"
)

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

func Envoyer(prix int, titre, desc, lien, img string, couleur int, source string) {
	webhook := config.WB_LUXE
	if prix <= 10000 {
		webhook = config.WB_0_10K
	} else if prix <= 20000 {
		webhook = config.WB_10_20K
	} else if prix <= 30000 {
		webhook = config.WB_20_30K
	} else if prix <= 50000 {
		webhook = config.WB_30_50K
	}

	if webhook == "" || strings.Contains(webhook, "LIEN_") {
		return
	}

	payload := DiscordMessage{
		Embeds: []DiscordEmbed{{
			Title: titre, Description: desc, URL: lien, Color: couleur,
			Image: DiscordImage{URL: img}, Footer: DiscordFooter{Text: "Source: " + source},
		}},
		Components: []DiscordComponent{{Type: 1, Components: []DiscordButton{{Type: 2, Label: "Voir l'annonce", Style: 5, URL: lien}}}},
	}
	jsonBody, _ := json.Marshal(payload)
	http.Post(webhook, "application/json", bytes.NewBuffer(jsonBody))
}

// Fonction pour le flux gratuit différé (optionnel, simplifié ici)
func EnvoyerGratuit(webhook, titre, desc, lien, img string) {
	payload := DiscordMessage{
		Embeds:     []DiscordEmbed{{Title: titre, Description: desc, URL: lien, Color: 9807270, Image: DiscordImage{URL: img}, Footer: DiscordFooter{Text: "Passez VIP"}}},
		Components: []DiscordComponent{{Type: 1, Components: []DiscordButton{{Type: 2, Label: "Voir", Style: 5, URL: lien}}}},
	}
	jsonBody, _ := json.Marshal(payload)
	http.Post(webhook, "application/json", bytes.NewBuffer(jsonBody))
}
