package scraper

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"sniper/config"
)

func LancerAutoScout() {
	// Profils navigateurs récents
	profils := []profiles.ClientProfile{
		profiles.Chrome_117,
		profiles.Safari_16_0,
		profiles.Firefox_110,
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
			time.Sleep(15 * time.Second)
			continue
		}

		scanPageAS(client)

		tempsPause := rand.Intn(30) + 45
		fmt.Printf("💤 [AutoScout] Pause %d sec...\n", tempsPause)
		time.Sleep(time.Duration(tempsPause) * time.Second)
		cycle++
	}
}

func scanPageAS(client tls_client.HttpClient) {
	// URL standard
	url := fmt.Sprintf("https://www.autoscout24.fr/lst?sort=age&desc=1&ustate=N%%2CU&size=20&page=1&cy=F&atype=C&fc=1&fregfrom=%d", config.ANNEE_MIN)

	req, _ := fhttp.NewRequest(fhttp.MethodGet, url, nil)
	req.Header = fhttp.Header{
		"User-Agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"},
		"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
		"Accept-Language": {"fr-FR,fr;q=0.9"},
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("❌ [AutoScout] Erreur connexion")
		return
	}
	defer resp.Body.Close()

	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(bodyBytes))

	// Sélecteur large
	selection := doc.Find("article, div[class*='ListItem_wrapper']")
	fmt.Printf("🧐 [AutoScout] %d éléments trouvés. Analyse des 3 premiers...\n", selection.Length())

	if selection.Length() == 0 {
		return
	}

	// On regarde seulement les 3 premiers pour ne pas spammer
	selection.Each(func(i int, s *goquery.Selection) {
		if i >= 3 {
			return
		} // Stop après 3

		fmt.Printf("\n📦 --- ELEMENT #%d ---\n", i+1)

		// 1. QUELS SONT LES LIENS ?
		fmt.Println("🔗 Liens trouvés dans cet élément :")
		foundLink := false
		s.Find("a").Each(func(k int, link *goquery.Selection) {
			href, _ := link.Attr("href")
			text := strings.TrimSpace(link.Text())
			if len(text) > 20 {
				text = text[:20] + "..."
			}
			fmt.Printf("   👉 [%s] -> %s\n", text, href)
			foundLink = true
		})
		if !foundLink {
			fmt.Println("   ⚠️ AUCUN LIEN <a> TROUVÉ !")
		}

		// 2. QUEL EST LE TEXTE ?
		// On nettoie les sauts de ligne pour que ce soit lisible
		rawText := strings.ReplaceAll(s.Text(), "\n", " ")
		rawText = strings.Join(strings.Fields(rawText), " ") // Retire les espaces multiples
		if len(rawText) > 150 {
			rawText = rawText[:150] + "..."
		}

		fmt.Printf("📝 Texte aperçu : %s\n", rawText)

		// 3. ATTRIBUTS DATA ?
		price := s.AttrOr("data-price", "NON")
		id := s.AttrOr("data-listing-id", "NON")
		fmt.Printf("💾 Data Attributes : Price=%s | ID=%s\n", price, id)
	})

	fmt.Println("\n🏁 Fin du diagnostic.")
}

// Petit utilitaire pour couper le texte trop long dans les logs
func textLimit(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
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

// Fonction locale pour l'analyse (identique à celle de lbc pour éviter les problèmes d'import)
func formatAnalyseAS(cote, prix int, src string, nb int, carb string) (string, int) {
	if cote == 0 || src == "Nouvelle Référence ✨" {
		return "🆕 **Nouvelle Référence !**", 9807270
	}
	marge := (float64(cote-prix) / float64(cote)) * 100
	detail := fmt.Sprintf("\n(Cote %s: %d€)", src, cote)
	if strings.Contains(src, "Argus") || strings.Contains(src, "Marché") {
		detail = fmt.Sprintf("\n(Basé sur %d véhicules)", nb)
	}

	if marge > 20 {
		return fmt.Sprintf("🔥 **SUPER DEAL!** (-%.0f%%)%s", marge, detail), 5763719
	}
	if marge > 5 {
		return fmt.Sprintf("✅ **Bonne affaire** (-%.0f%%)%s", marge, detail), 15844367
	}
	if marge < -10 {
		return fmt.Sprintf("❌ **Trop cher** %s", detail), 15548997
	}
	return fmt.Sprintf("😐 **Prix marché**%s", detail), 3447003
}
