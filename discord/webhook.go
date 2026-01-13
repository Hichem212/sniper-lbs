package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sniper/config"
	"strings"
	"time"
)

// --- STRUCTURES DISCORD ---
// On ajoute les "Fields" et "Timestamp" pour le style Vinted
type DiscordMessage struct {
	Embeds     []DiscordEmbed     `json:"embeds"`
	Components []DiscordComponent `json:"components"`
}

type DiscordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"` // On le garde au cas où
	URL         string         `json:"url"`
	Color       int            `json:"color"`
	Fields      []DiscordField `json:"fields,omitempty"` // C'est ici que la magie opère
	Image       DiscordImage   `json:"image,omitempty"`
	Footer      DiscordFooter  `json:"footer,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
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

// --- FONCTION D'ENVOI INTELLIGENTE ---
// On garde EXACTEMENT la même signature qu'avant pour ne rien casser
func Envoyer(cote, prix int, titre, desc, lien, img string, couleur int, source string) {

	// 1. EXTRACTION DES DONNÉES (Parsing)
	// Puisqu'on reçoit tout dans "desc", on va aller pêcher les infos nous-mêmes
	// Format reçu habituel : "🗓️ **Année :** 2018"

	reAnnee := regexp.MustCompile(`Année :[*\s]+(\d+)`)
	reKm := regexp.MustCompile(`Km :[*\s]+(\d+)`)
	reCarb := regexp.MustCompile(`Énergie :[*\s]+([^\n]+)`)
	reVille := regexp.MustCompile(`Ville :[*\s]+([^\n]+)`)

	annee := "????"
	km := "????"
	carburant := "Autre"
	ville := "France"

	if m := reAnnee.FindStringSubmatch(desc); len(m) > 1 {
		annee = m[1]
	}
	if m := reKm.FindStringSubmatch(desc); len(m) > 1 {
		km = m[1]
	}
	if m := reCarb.FindStringSubmatch(desc); len(m) > 1 {
		carburant = strings.TrimSpace(m[1])
	}
	if m := reVille.FindStringSubmatch(desc); len(m) > 1 {
		ville = strings.TrimSpace(m[1])
	}

	// 2. ANALYSE FINANCIÈRE (Recalculée pour être sûr d'avoir le bon code couleur)
	marge := 0.0
	iconEtat := "🚗"
	analyseTxt := "⚖️ **Prix de marché** (Standard)"

	webhook := config.WB_LUXE // Par défaut
	finalColor := couleur     // On part sur la couleur donnée par le scraper

	if cote > 0 {
		marge = (float64(cote-prix) / float64(cote)) * 100
		gain := cote - prix

		if marge >= 20 {
			webhook = config.WB_ADMIN
			finalColor = 15844367 // OR (Gold) -> Super affaire
			iconEtat = "🔥"
			analyseTxt = fmt.Sprintf("🚀 **EXCELLENTE AFFAIRE**\n📉 Cote: %d €\n💰 **Gain: %d €** (+%.0f%%)", cote, gain, marge)
		} else if marge >= 10 {
			finalColor = 3066993 // VERT
			iconEtat = "✅"
			analyseTxt = fmt.Sprintf("✅ **TRÈS BON PRIX**\n📉 Cote: %d €\n💸 Gain: %d €", cote, gain)
		} else if marge <= -10 {
			finalColor = 15158332 // ROUGE
			iconEtat = "⚠️"
			analyseTxt = fmt.Sprintf("⚠️ **Au-dessus de la cote**\n📉 Cote: %d €\n❌ Surcoût: %d €", cote, prix-cote)
		}
	}

	// Routage Webhook (Budget)
	if marge < 20 {
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

	if webhook == "" || strings.Contains(webhook, "WEBHOOK_") {
		return
	}

	// 3. CONSTRUCTION DU VISUEL "VINTED / PRO"

	// Nettoyage source (Enlever "Argus Expert B...")
	sourceClean := strings.Split(source, "(")[0]
	sourceClean = strings.TrimSpace(sourceClean)

	// Ligne 1 : Caractéristiques Clés
	infosLigne := fmt.Sprintf("**%d €** • %s • %s km", prix, annee, km)

	// Ligne 2 : Détails
	detailsLigne := fmt.Sprintf("⛽ %s   |   📍 %s", carburant, ville)

	// Création des Champs (Fields)
	fields := []DiscordField{
		{
			Name:   "🏁 Caractéristiques",
			Value:  infosLigne,
			Inline: false,
		},
		{
			Name:   "⚙️ Infos",
			Value:  detailsLigne,
			Inline: false,
		},
	}

	// Ajout du bloc Analyse si cote dispo
	if cote > 0 {
		fields = append(fields, DiscordField{
			Name:   "📊 Analyse Financière",
			Value:  analyseTxt,
			Inline: false,
		})
	}

	// 4. PAYLOAD FINAL
	payload := DiscordMessage{
		Embeds: []DiscordEmbed{{
			Title:  fmt.Sprintf("%s %s", iconEtat, titre),
			URL:    lien,
			Color:  finalColor,
			Fields: fields, // On utilise nos jolis champs ici
			Image:  DiscordImage{URL: img},
			//Footer:    DiscordFooter{Text: "Sniper Auto • " + sourceClean},
			Timestamp: time.Now().Format(time.RFC3339),
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

// Fonction pour le flux gratuit (Simplifiée)
func EnvoyerGratuit(webhook, titre, desc, lien, img string) {
	// Pour le gratuit, on garde l'affichage simple ou on l'adapte légèrement
	payload := DiscordMessage{
		Embeds: []DiscordEmbed{{
			Title:       titre,
			Description: desc,
			URL:         lien,
			Color:       9807270,
			Image:       DiscordImage{URL: img},
			Footer:      DiscordFooter{Text: "💎 Passez VIP pour avoir les alertes en temps réel"},
		}},
		Components: []DiscordComponent{{Type: 1, Components: []DiscordButton{{Type: 2, Label: "Voir", Style: 5, URL: lien}}}},
	}
	jsonBody, _ := json.Marshal(payload)
	http.Post(webhook, "application/json", bytes.NewBuffer(jsonBody))
}
