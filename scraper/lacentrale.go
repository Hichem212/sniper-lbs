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

// ⬇️ ⬇️ COLLE TON COOKIE GÉANT ENTRE LES GUILLEMETS ICI ⬇️ ⬇️
const CONST_COOKIE = "datadome=.............; _ga=............."

func LancerLC() {
	// On utilise Chrome 120 pour correspondre aux headers
	profil := profiles.Chrome_120

	cycle := 1
	for {
		fmt.Printf("\n🔵 [LaCentrale] Cycle #%d...\n", cycle)

		jar := tls_client.NewCookieJar()
		options := []tls_client.HttpClientOption{
			tls_client.WithTimeoutSeconds(30),
			tls_client.WithClientProfile(profil),
			tls_client.WithNotFollowRedirects(),
			tls_client.WithCookieJar(jar),
		}
		client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)

		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}

		scanPageLC(client)

		// Pause aléatoire
		pause := rand.Intn(30) + 40
		fmt.Printf("💤 Pause LaCentrale %d sec...\n", pause)
		time.Sleep(time.Duration(pause) * time.Second)
		cycle++
	}
}

func scanPageLC(client tls_client.HttpClient) {
	url := "https://www.lacentrale.fr/listing?sorting=CREATION_DATE_DESC"

	req, _ := fhttp.NewRequest(fhttp.MethodGet, url, nil)

	// HEADERS AVEC LE COOKIE MAGIQUE
	req.Header = fhttp.Header{
		"authority":                 {"www.lacentrale.fr"},
		"accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"},
		"accept-language":           {"fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7"},
		"cache-control":             {"max-age=0"},
		"cookie":                    {CONST_COOKIE}, // <--- C'EST ICI QUE LA MAGIE OPÈRE
		"referer":                   {"https://www.lacentrale.fr/"},
		"sec-ch-ua":                 {`"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`},
		"sec-ch-ua-mobile":          {"?0"},
		"sec-ch-ua-platform":        {`"Windows"`},
		"sec-fetch-dest":            {"document"},
		"sec-fetch-mode":            {"navigate"},
		"sec-fetch-site":            {"same-origin"},
		"sec-fetch-user":            {"?1"},
		"upgrade-insecure-requests": {"1"},
		"user-agent":                {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
	}

	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("⛔ BLOCAGE LC Code: %d (Ton cookie est peut-être périmé ?)\n", resp.StatusCode)
		return
	}

	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))

	rePrix := regexp.MustCompile(`([0-9]{1,3}(?:[\s]\d{3})*)\s*€`)
	reAnnee := regexp.MustCompile(`\b(20[0-2][0-9])\b`)
	reKm := regexp.MustCompile(`(\d{1,3}(?:[\s]\d{3})*)\s*km`)

	compteur := 0

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		lien, _ := s.Attr("href")
		if !strings.Contains(lien, "auto-occasion-annonce") {
			return
		}

		lien = "https://www.lacentrale.fr" + lien
		parts := strings.Split(lien, "-")
		if len(parts) < 2 {
			return
		}
		id := strings.ReplaceAll(parts[len(parts)-1], ".html", "")

		if database.Exists(id, "LaCentrale") {
			return
		}

		fullText := s.Text()
		if len(fullText) < 10 {
			fullText = s.Parent().Text()
		}
		fullText = strings.ReplaceAll(fullText, "\n", " ")

		titre := strings.TrimSpace(s.Find("h3").Text())
		if titre == "" {
			if len(fullText) > 30 {
				titre = fullText[:30] + "..."
			} else {
				titre = fullText
			}
		}

		prixInt := 0
		if m := rePrix.FindStringSubmatch(fullText); len(m) > 1 {
			fmt.Sscanf(strings.ReplaceAll(m[1], " ", ""), "%d", &prixInt)
		}

		anneeInt := 0
		if m := reAnnee.FindStringSubmatch(fullText); len(m) > 1 {
			fmt.Sscanf(m[1], "%d", &anneeInt)
		}

		kmInt := 0
		if m := reKm.FindStringSubmatch(fullText); len(m) > 1 {
			fmt.Sscanf(strings.ReplaceAll(m[1], " ", ""), "%d", &kmInt)
		}

		if anneeInt < config.ANNEE_MIN || prixInt < 500 {
			return
		}

		carburant := detectCarbLC(fullText)
		cote, source, nb := brain.EstimerPrix(titre, anneeInt, kmInt, carburant, prixInt)
		analyse, coul := formatAnalyseLC(cote, prixInt, source, nb, carburant)

		kmDisp := fmt.Sprintf("%d km", kmInt)
		desc := fmt.Sprintf("**Prix:** %d €\n**Année:** %d\n**Km:** %s\n⛽ %s\n\n%s", prixInt, anneeInt, kmDisp, carburant, analyse)

		discord.Envoyer(prixInt, "🔵 "+titre, desc, lien, s.Find("img").AttrOr("src", ""), coul, "LaCentrale • "+source)
		database.InsertAnnonce(id, "LaCentrale", titre, prixInt, anneeInt, kmInt, carburant, "France", cote, lien, s.Find("img").AttrOr("src", ""))

		fmt.Printf("🔵 LC | %s | %d€\n", titre, prixInt)
		compteur++
	})

	if compteur == 0 {
		fmt.Println("⚠️  Page chargée mais 0 annonce (Vérifie si la page reçue n'est pas un captcha).")
	} else {
		fmt.Printf("   🔵 %d annonces trouvées.\n", compteur)
	}
}

func detectCarbLC(t string) string {
	t = strings.ToLower(t)
	if strings.Contains(t, "électrique") {
		return "Électrique"
	}
	if strings.Contains(t, "hybride") {
		return "Hybride"
	}
	if strings.Contains(t, "diesel") {
		return "Diesel"
	}
	return "Essence"
}

func formatAnalyseLC(cote, prix int, src string, nb int, carb string) (string, int) {
	if cote == 0 {
		return "🆕 **Nouvelle Référence !**", 9807270
	}
	marge := (float64(cote-prix) / float64(cote)) * 100
	detail := fmt.Sprintf("\n(Cote %s: %d€)", src, cote)
	if marge > 20 {
		return fmt.Sprintf("🔥 **SUPER DEAL!** (+%.0f%%)%s", marge, detail), 5763719
	}
	if marge > 5 {
		return fmt.Sprintf("✅ **Bonne affaire**%s", detail), 15844367
	}
	return fmt.Sprintf("😐 **Prix marché**%s", detail), 3447003
}
