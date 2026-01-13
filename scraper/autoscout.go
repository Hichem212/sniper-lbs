package scraper

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"regexp"
	"strconv"
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

// LancerAutoScout : Boucle principale
func LancerAutoScout() {
	// On fixe un profil récent pour correspondre aux headers
	profils := []profiles.ClientProfile{
		profiles.Chrome_117,
	}
	cycle := 1

	for {
		fmt.Printf("\n🇪🇺 [AutoScout] Cycle #%d - Scan...\n", cycle)

		jar := tls_client.NewCookieJar()
		options := []tls_client.HttpClientOption{
			tls_client.WithTimeoutSeconds(30),
			tls_client.WithClientProfile(profils[rand.Intn(len(profils))]),
			tls_client.WithNotFollowRedirects(),
			tls_client.WithCookieJar(jar),
		}
		client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

		if err != nil {
			fmt.Println("❌ Erreur création client TLS:", err)
			time.Sleep(10 * time.Second)
			continue
		}

		scanPageAS(client)

		// Pause aléatoire
		tempsPause := rand.Intn(30) + 45
		fmt.Printf("💤 [AutoScout] Pause %d sec...\n", tempsPause)
		time.Sleep(time.Duration(tempsPause) * time.Second)
		cycle++
	}
}

// scanPageAS : Analyse de la page (MODE DEBUG ACTIVÉ)
func scanPageAS(client tls_client.HttpClient) {
	// URL
	url := fmt.Sprintf("https://www.autoscout24.fr/lst?sort=age&desc=1&ustate=N%%2CU&size=20&page=1&cy=F&atype=C&fc=1&fregfrom=%d", config.ANNEE_MIN)

	req, _ := fhttp.NewRequest(fhttp.MethodGet, url, nil)

	// HEADERS
	req.Header = fhttp.Header{
		"authority":                 {"www.autoscout24.fr"},
		"accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
		"accept-language":           {"fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7"},
		"cache-control":             {"max-age=0"},
		"sec-ch-ua":                 {`"Google Chrome";v="117", "Not;A=Brand";v="8", "Chromium";v="117"`},
		"sec-ch-ua-mobile":          {"?0"},
		"sec-ch-ua-platform":        {`"Windows"`},
		"sec-fetch-dest":            {"document"},
		"sec-fetch-mode":            {"navigate"},
		"sec-fetch-site":            {"none"},
		"sec-fetch-user":            {"?1"},
		"upgrade-insecure-requests": {"1"},
		"user-agent":                {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36"},
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ Erreur requête:", err)
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))

	// DEBUG CHECK
	titrePage := doc.Find("title").Text()
	if strings.Contains(titrePage, "Security") || strings.Contains(titrePage, "Just a moment") {
		fmt.Printf("⚠️  BLOQUÉ CLOUDFLARE. Titre: %s\n", titrePage)
		return
	}

	// Sélecteur adapté à ton HTML (DeclutteredListItem)
	selection := doc.Find("article, div[class*='ListItem_wrapper'], div[class*='DeclutteredListItem_container']")

	if selection.Length() == 0 {
		fmt.Println("⚠️  Aucune annonce trouvée (Sélecteur vide).")
		return
	} else {
		fmt.Printf("🔎 Analyse de %d éléments...\n", selection.Length())
	}

	// Regex UUID
	reUUID := regexp.MustCompile(`(?i)[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`)
	rePrix := regexp.MustCompile(`€\s?([0-9\s\.]+)`)
	reKm := regexp.MustCompile(`([0-9\s\.]+)\s?km`)
	reAnnee := regexp.MustCompile(`(0[1-9]|1[0-2])/(20[0-9]{2})`)

	compteur := 0

	selection.Each(func(i int, s *goquery.Selection) {
		texteComplet := s.Text()

		// 1. EXTRACTION BASIQUE
		prixStr := s.AttrOr("data-price", "")
		if prixStr == "" {
			// Essai dans le HTML spécifique que tu m'as montré
			prixStr = s.Find("span[class*='CurrentPrice_price']").Text()
			if prixStr == "" {
				match := rePrix.FindStringSubmatch(texteComplet)
				if len(match) > 1 {
					prixStr = match[1]
				}
			}
		}
		// Nettoyage radical du prix
		prixStr = strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, prixStr)
		prixInt, _ := strconv.Atoi(prixStr)

		// Extraction KM et Année via Regex sur le texte car les attributs manquent parfois
		kmInt := 0
		matchKm := reKm.FindStringSubmatch(strings.ReplaceAll(texteComplet, "\u202f", "")) // Espace insécable
		if len(matchKm) > 1 {
			rawKm := strings.ReplaceAll(matchKm[1], " ", "")
			rawKm = strings.ReplaceAll(rawKm, ".", "")
			kmInt, _ = strconv.Atoi(rawKm)
		}

		anneeInt := 0
		matchAnnee := reAnnee.FindStringSubmatch(texteComplet)
		if len(matchAnnee) > 2 {
			anneeInt, _ = strconv.Atoi(matchAnnee[2])
		}

		// Titre
		titre := s.Find("h2").Text()
		titre = strings.ReplaceAll(titre, "\\u0026", "&")

		// Filtres
		if prixInt == 0 || anneeInt < config.ANNEE_MIN {
			return
		}

		// ---------------------------------------------------------
		// 2. RECUPERATION DU LIEN (LA CORRECTION)
		// ---------------------------------------------------------
		var lien, id string

		// ÉTAPE 1 : Chercher un lien classique (souvent absent maintenant)
		s.Find("a").Each(func(_ int, link *goquery.Selection) {
			href, exists := link.Attr("href")
			if exists && strings.Contains(href, "/offres/") {
				lien = href
				return
			}
		})

		// ÉTAPE 2 (CRITIQUE) : Si pas de lien, on vole l'ID dans l'URL de l'IMAGE
		if lien == "" {
			// On prend l'image principale
			imgSrc := s.Find("img").AttrOr("src", "")
			if imgSrc == "" {
				// Parfois c'est dans <source srcset="...">
				imgSrc = s.Find("source").AttrOr("srcset", "")
			}

			// On cherche l'UUID dans l'URL de l'image
			if reUUID.MatchString(imgSrc) {
				id = reUUID.FindString(imgSrc)
				// BINGO ! On reconstruit le lien manuellement
				lien = "https://www.autoscout24.fr/offres/" + id
			}
		}

		// Si on a trouvé un lien mais pas encore extrait l'ID
		if id == "" && lien != "" {
			if reUUID.MatchString(lien) {
				id = reUUID.FindString(lien)
			}
		}

		// ÉTAPE 3 : Si échec total (pas de lien, pas d'image avec ID)
		if id == "" {
			// On skip l'annonce car impossible de générer un lien valide
			// fmt.Println("      ❌ Impossible de trouver l'ID (ni dans le lien, ni dans l'image)")
			return
		}

		// Nettoyage final du lien
		if !strings.HasPrefix(lien, "http") {
			lien = "https://www.autoscout24.fr" + lien
		}
		// ---------------------------------------------------------

		if database.Exists(id, "AutoScout") {
			return
		}

		// 3. SUITE DU TRAITEMENT...
		carburant := detectCarbAS(texteComplet)

		ville := s.Find("span[class*='ListItemSeller_address']").Text()
		if ville == "" {
			ville = "France"
		} else {
			// Nettoyage: "FR-69000 Lyon" -> "Lyon" (Approximation)
			ville = strings.TrimPrefix(ville, "FR-")
			parts := strings.Fields(ville)
			if len(parts) > 1 {
				ville = strings.Join(parts[1:], " ") // Enlève le code postal souvent au début
			}
		}

		// Image URL (On l'a déjà peut-être récupérée pour l'ID)
		imageURL := s.Find("img").AttrOr("src", "")
		if imageURL == "" {
			imageURL = s.Find("source").First().AttrOr("srcset", "")
		}

		cote, source, nbData := brain.EstimerPrix(titre, anneeInt, kmInt, carburant, prixInt)

		couleur := 9807270
		marge := 0.0

		if cote > 0 {
			marge = (float64(cote-prixInt) / float64(cote)) * 100
			if marge >= 20 {
				couleur = 5763719
			} else if marge <= -10 {
				couleur = 15548997
			}
		}

		ligneCote := "🚫 Pas de cote disponible"
		if cote > 0 {
			ligneCote = fmt.Sprintf("📉 **Cote Argus :** %d €", cote)
			if marge > 5 {
				ligneCote += fmt.Sprintf("\n🔥 **Marge :** -%.0f%% (Gain: %d€)", marge, cote-prixInt)
			}
		}

		desc := fmt.Sprintf(
			"💰 **PRIX : %d €**\n\n"+
				"🗓️ **Année :** %d\n"+
				"📏 **Km :** %d km\n"+
				"⛽ **Énergie :** %s\n"+
				"📍 **Ville :** %s\n\n"+
				"%s",
			prixInt, anneeInt, kmInt, carburant, ville, ligneCote,
		)

		sourceInfos := fmt.Sprintf("%s (%d annonces)", source, nbData)

		discord.Envoyer(cote, prixInt, titre, desc, lien, imageURL, couleur, sourceInfos)
		database.InsertAnnonce(id, "AutoScout", titre, prixInt, anneeInt, kmInt, carburant, ville, cote, lien, imageURL)

		fmt.Printf("🚀 [AS] VALIDÉ (ID:%s) : %s | %d€\n", id[:6], titre, prixInt)
		compteur++
	})

	if compteur > 0 {
		fmt.Printf("   ✅ %d annonces ajoutées.\n", compteur)
	}
}

// detectCarbAS : Détection multilingue pour AutoScout
func detectCarbAS(t string) string {
	t = strings.ToLower(t)
	if strings.Contains(t, "électrique") || strings.Contains(t, "electric") {
		return "Électrique"
	}
	if strings.Contains(t, "hybride") || strings.Contains(t, "hybrid") {
		return "Hybride"
	}
	if strings.Contains(t, "diesel") {
		return "Diesel"
	}
	return "Essence"
}
