package scraper

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	// Import des autres dossiers de ton projet
	"sniper/brain"
	"sniper/config"
	"sniper/database"
	"sniper/discord"
)

func Lancer() {
	profils := []profiles.ClientProfile{profiles.Chrome_117, profiles.Firefox_110, profiles.Opera_90}
	cycle := 1
	for {
		fmt.Printf("\n🕵️ Cycle #%d - Scan complet...\n", cycle)

		// Création du client HTTP
		jar := tls_client.NewCookieJar()
		options := []tls_client.HttpClientOption{
			tls_client.WithTimeoutSeconds(30),
			tls_client.WithClientProfile(profils[rand.Intn(len(profils))]),
			tls_client.WithNotFollowRedirects(),
			tls_client.WithCookieJar(jar),
			tls_client.WithRandomTLSExtensionOrder(),
		}
		client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}

		scanPage(client)

		tempsPause := rand.Intn(20) + 30
		fmt.Printf("💤 Pause %d sec...\n", tempsPause)
		time.Sleep(time.Duration(tempsPause) * time.Second)
		cycle++
	}
}

func scanPage(client tls_client.HttpClient) {
	url := "https://www.leboncoin.fr/recherche?category=2&sort=time"
	req, _ := fhttp.NewRequest(fhttp.MethodGet, url, nil)
	req.Header = fhttp.Header{
		"User-Agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
		"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
		"Accept-Language": {"fr-FR,fr;q=0.9"},
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))

	reAnnee := regexp.MustCompile(`\b(19|20)[0-9]{2}\b`)
	// Regex permissive pour les KM (avec ou sans espace, avec ou sans point)
	reKm := regexp.MustCompile(`(?i)(\d{1,3}(?:[\s\.]?\d{3})*)\s*km`)

	compteur := 0

	doc.Find("article[data-test-id='ad']").Each(func(i int, s *goquery.Selection) {
		lien, _ := s.Find("a").Attr("href")
		if lien == "" {
			return
		}
		if !strings.HasPrefix(lien, "http") {
			lien = "https://www.leboncoin.fr" + lien
		}
		id := strings.Split(lien, "/")[len(strings.Split(lien, "/"))-1]

		// 1. Appel au package DATABASE
		if database.Exists(id, "Leboncoin") {
			return
		}

		titre := s.Find("[data-test-id='adcard-title']").Text()
		prixStr := s.Find("[data-test-id='price']").Text()

		// Appel fonction locale cleanPrix
		prixInt := cleanPrix(prixStr)

		texteCarte := s.Text()

		anneeInt := 0
		if match := reAnnee.FindString(texteCarte); match != "" {
			fmt.Sscanf(match, "%d", &anneeInt)
		}

		kmInt := 0
		matchKm := reKm.FindStringSubmatch(texteCarte)
		if len(matchKm) > 1 {
			// Nettoyage : on enlève espaces et points
			rawKm := strings.ReplaceAll(matchKm[1], " ", "")
			rawKm = strings.ReplaceAll(rawKm, ".", "")
			rawKm = strings.ReplaceAll(rawKm, "\u00a0", "")
			fmt.Sscanf(rawKm, "%d", &kmInt)
		}

		// Appel fonction locale detectCarb
		carburant := detectCarb(texteCarte)

		// 2. Appel au package CONFIG
		if anneeInt < config.ANNEE_MIN || anneeInt > config.ANNEE_MAX || titre == "" || prixInt == 0 {
			return
		}

		ville := "France"
		s.Find("p").Each(func(k int, p *goquery.Selection) {
			if strings.Contains(p.Text(), "Située à") {
				ville = strings.TrimSpace(strings.ReplaceAll(p.Text(), "Située à", ""))
			}
		})
		imageURL := s.Find("img").AttrOr("src", "")

		// 3. Appel au package BRAIN (Cerveau)
		cote, source, nbData := brain.EstimerPrix(titre, anneeInt, kmInt, carburant, prixInt)

		// Affichage et analyse
		analyse, couleur := formatAnalyse(cote, prixInt, source, nbData, carburant)

		kmDisp := "N/A"
		if kmInt > 0 {
			kmDisp = fmt.Sprintf("%d km", kmInt)
		}

		iconeCarb := "⛽"
		if carburant == "Électrique" {
			iconeCarb = "⚡"
		} else if carburant == "Hybride" {
			iconeCarb = "🔋"
		}

		desc := fmt.Sprintf("**Prix:** %d €\n**Année:** %d\n**Km:** %s\n%s **%s**\n📍 %s\n\n%s", prixInt, anneeInt, kmDisp, iconeCarb, carburant, ville, analyse)

		// 4. Appel au package DISCORD
		discord.Envoyer(prixInt, "⚡ "+titre, desc, lien, imageURL, couleur, source)

		// 5. Appel au package DATABASE (Sauvegarde)
		// On ajoute "Leboncoin" comme 2ème argument
		database.InsertAnnonce(id, "Leboncoin", titre, prixInt, anneeInt, kmInt, carburant, ville, cote, lien, imageURL)
		fmt.Printf("🚀 %s | %d km | %d€\n", titre, kmInt, prixInt)
		compteur++
	})
	if compteur > 0 {
		fmt.Printf("   🔎 %d annonces traitées.\n", compteur)
	}
}

// --- FONCTIONS LOCALES (Utilitaires propres au scraper) ---

func cleanPrix(p string) int {
	reg := regexp.MustCompile("[^0-9]+")
	var v int
	fmt.Sscanf(reg.ReplaceAllString(p, ""), "%d", &v)
	return v
}

func detectCarb(t string) string {
	t = strings.ToLower(t)
	if strings.Contains(t, "électrique") || strings.Contains(t, "tesla") || strings.Contains(t, "élec") {
		return "Électrique"
	}
	if strings.Contains(t, "hybride") {
		return "Hybride"
	}
	if strings.Contains(t, "diesel") || strings.Contains(t, "hdi") {
		return "Diesel"
	}
	return "Essence"
}

func formatAnalyse(cote, prix int, src string, nb int, carb string) (string, int) {
	if cote == 0 || src == "Nouvelle Référence ✨" {
		return "🆕 **Nouvelle Référence !**", 9807270
	}

	marge := (float64(cote-prix) / float64(cote)) * 100
	detail := fmt.Sprintf("\n(Cote %s: %d€)", src, cote)
	if src == "Médiane Précise 🎯" {
		detail = fmt.Sprintf("\n(Comparé avec %d véhicules )", nb)
	}

	if marge > 20 {
		return fmt.Sprintf("🔥 **SUPER DEAL!** (-%.0f%%)%s", marge, detail), 5763719
	}
	if marge > 5 {
		return fmt.Sprintf("✅ **Bonne affaire**(-%.0f%%)%s", marge, detail), 15844367
	}
	if marge < -10 {
		return fmt.Sprintf("❌ **Trop cher**(%.0f%%)%s", marge, detail), 15548997
	}
	return fmt.Sprintf("😐 **Prix marché**%s", detail), 3447003
}
