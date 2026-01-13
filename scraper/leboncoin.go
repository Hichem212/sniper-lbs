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

	"sniper/brain"
	"sniper/config"
	"sniper/database"
	"sniper/discord"
)

// Lancer : Boucle principale du scraper Leboncoin
func Lancer() {
	// Rotation de profils pour éviter le blocage
	profils := []profiles.ClientProfile{
		profiles.Chrome_117,
		profiles.Firefox_110,
		profiles.Opera_90,
	}

	cycle := 1
	for {
		fmt.Printf("\n🇫🇷 [Leboncoin] Cycle #%d - Scan...\n", cycle)

		// Configuration du client TLS (Anti-Bot)
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

		// Pause aléatoire humaine (entre 30s et 50s)
		tempsPause := rand.Intn(20) + 30
		fmt.Printf("💤 [Leboncoin] Pause %d sec...\n", tempsPause)
		time.Sleep(time.Duration(tempsPause) * time.Second)
		cycle++
	}
}

// scanPage : Analyse la page de recherche
func scanPage(client tls_client.HttpClient) {
	// URL : Trié par date (les plus récentes en premier)
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

	// Regex pour extraire les données brutes
	reAnnee := regexp.MustCompile(`\b(19|20)[0-9]{2}\b`)
	reKm := regexp.MustCompile(`(?i)(\d{1,3}(?:[\s\.]?\d{3})*)\s*km`)

	compteur := 0

	// Sélecteur des annonces
	doc.Find("article[data-test-id='ad']").Each(func(i int, s *goquery.Selection) {

		// 1. LIEN & ID
		lien, _ := s.Find("a").Attr("href")
		if lien == "" {
			return
		}
		if !strings.HasPrefix(lien, "http") {
			lien = "https://www.leboncoin.fr" + lien
		}

		// Extraction ID unique (la fin de l'URL)
		parts := strings.Split(lien, "/")
		id := parts[len(parts)-1]
		id = strings.Split(id, ".")[0] // Enlève le .htm éventuel

		// Si déjà vu, on passe
		if database.Exists(id, "Leboncoin") {
			return
		}

		// 2. EXTRACTION DONNÉES
		titre := s.Find("[data-test-id='adcard-title']").Text()
		prixStr := s.Find("[data-test-id='price']").Text()
		prixInt := cleanPrix(prixStr)

		texteCarte := s.Text()

		// Année
		anneeInt := 0
		if match := reAnnee.FindString(texteCarte); match != "" {
			fmt.Sscanf(match, "%d", &anneeInt)
		}

		// Km
		kmInt := 0
		matchKm := reKm.FindStringSubmatch(texteCarte)
		if len(matchKm) > 1 {
			rawKm := strings.ReplaceAll(matchKm[1], " ", "")
			rawKm = strings.ReplaceAll(rawKm, ".", "")
			rawKm = strings.ReplaceAll(rawKm, "\u00a0", "") // Espace insécable
			fmt.Sscanf(rawKm, "%d", &kmInt)
		}

		// Carburant
		carburant := detectCarb(texteCarte)

		// Filtres de sécurité
		if anneeInt < config.ANNEE_MIN || titre == "" || prixInt == 0 {
			return
		}

		// Ville
		ville := "France"
		s.Find("p").Each(func(k int, p *goquery.Selection) {
			if strings.Contains(p.Text(), "Située à") {
				ville = strings.TrimSpace(strings.ReplaceAll(p.Text(), "Située à", ""))
			}
		})

		// Image
		imageURL := s.Find("img").AttrOr("src", "")

		// 3. CERVEAU & INTELLIGENCE
		cote, source, nbData := brain.EstimerPrix(titre, anneeInt, kmInt, carburant, prixInt)

		// 4. PRÉPARATION DU VISUEL (COULEUR & TEXTE)
		couleur := 9807270 // Gris (Neutre)
		marge := 0.0

		if cote > 0 {
			marge = (float64(cote-prixInt) / float64(cote)) * 100

			// Code Couleur pour Discord
			if marge >= 20 {
				couleur = 5763719 // VERT (Super affaire)
			} else if marge <= -10 {
				couleur = 15548997 // ROUGE (Trop cher)
			}
		}

		// Construction de la ligne Cote "Propre"
		ligneCote := "🚫 Pas de cote disponible"
		if cote > 0 {
			ligneCote = fmt.Sprintf("📉 **Cote Argus :** %d €", cote)
			// On n'affiche la marge dans le texte que si elle est significative
			if marge > 5 {
				ligneCote += fmt.Sprintf("\n🔥 **Marge :** -%.0f%% (Gain: %d€)", marge, cote-prixInt)
			}
			if marge < 0 {
				ligneCote += fmt.Sprintf("\n⚠️ **Au-dessus de la cote :** +%.0f%% (Surcoût: %d€)", -marge, prixInt-cote)

			}
		}

		// Description style "Fiche Technique"
		desc := fmt.Sprintf(
			"💰 **PRIX : %d €**\n\n"+
				"🗓️ **Année :** %d\n"+
				"📏 **Km :** %d km\n"+
				"⛽ **Énergie :** %s\n"+
				"📍 **Ville :** %s\n\n"+
				"%s", // Ici on insère la ligne Cote propre
			prixInt, anneeInt, kmInt, carburant, ville, ligneCote,
		)

		// Info cachée pour le footer (basé sur x annonces)
		sourceInfos := fmt.Sprintf("%s (%d annonces)", source, nbData)

		// 5. ENVOI DISCORD
		// On passe toutes les infos, c'est discord.Envoyer qui décidera du salon (Admin ou Public)
		discord.Envoyer(cote, prixInt, titre, desc, lien, imageURL, couleur, sourceInfos)

		// 6. SAUVEGARDE DB
		database.InsertAnnonce(id, "Leboncoin", titre, prixInt, anneeInt, kmInt, carburant, ville, cote, lien, imageURL)

		fmt.Printf("🚀 [LBC] %s | %d€ | Marge: %.0f%%\n", titre, prixInt, marge)
		compteur++
	})

	if compteur > 0 {
		fmt.Printf("   ✅ %d annonces traitées.\n", compteur)
	}
}

// --- UTILITAIRES ---

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
