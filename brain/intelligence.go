package brain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"sniper/config"
	"sniper/database"
)

// Structure pour manipuler les données proprement
type Comparable struct {
	Prix  int
	Km    int
	Annee int
	Date  time.Time
}

// --- FONCTION PRINCIPALE ---

func EstimerPrix(titre string, annee, km int, carb string, prixActuel int) (int, string, int) {

	// 1. NORMALISATION & SÉCURITÉ
	// Nettoyage du titre + Vérification des "Mots Fatals" avec gestion des négations
	pattern, estValide := normaliserTitre(titre)
	if !estValide {
		return prixActuel, "Ignoré (HS/Panne/Export) 🗑️", 0
	}

	// 2. DATA MINING (SQL)
	// On récupère les données brutes sur +/- 2 ans
	rawData := getComparables(pattern, annee, km, carb)

	// 3. STATISTIQUES & FILTRAGE
	// Suppression des valeurs aberrantes (Outliers)
	cleanData := filtrerOutliers(rawData)
	nbData := len(cleanData)

	// Il nous faut un minimum de matière pour faire de la stat
	if nbData >= 5 {

		// A. Tendance Marché (Hausse/Baisse sur 30 jours)
		tendance := calculerTendance(cleanData)

		// B. Construction du VÉHICULE DE RÉFÉRENCE
		// On calcule le prix et le km moyens pondérés par la similarité
		prixRef, kmRef := calculerMoyennesPonderees(cleanData, annee, km)

		// C. AJUSTEMENT DIFFÉRENTIEL (Le cœur du moteur)
		// On ajuste ce prix de référence selon tes spécificités (Delta Km, Année, Carburant)
		coteFinale := ajusterPrixIndustriel(prixRef, kmRef, annee, km, carb, tendance)

		// D. Score de Confiance
		confiance := calculerConfiance(nbData, cleanData, coteFinale)

		label := fmt.Sprintf("Argus Expert %s (%s)", confiance, tendance)
		return coteFinale, label, nbData
	}

	// 4. FALLBACK IA SÉCURISÉ (Si pas assez de data SQL)
	fmt.Print("⏳")
	time.Sleep(2 * time.Second)
	estIA := demanderGemini(titre, annee, km, carb, prixActuel)

	if estIA > 0 {
		// Bornage de sécurité IA (+/- 40%)
		minSecu := float64(prixActuel) * 0.60
		maxSecu := float64(prixActuel) * 1.40
		if float64(estIA) < minSecu || float64(estIA) > maxSecu {
			return prixActuel, "IA (Hors Limites) ⚠️", 0
		}
		return estIA, "IA Gemini 🤖", 0
	}

	return prixActuel, "Nouvelle Référence ✨", 1
}

// --- 1. NORMALISATION & INTELLIGENCE SÉMANTIQUE ---

func normaliserTitre(titre string) (string, bool) {
	titreBrut := strings.ToLower(titre)

	// LISTE NOIRE INTELLIGENTE
	// On rejette l'annonce si elle contient ces mots, SAUF s'ils sont précédés de "pas", "sans", "aucun"...
	motsFatals := []string{"hs", "panne", "probleme", "accident", "carte grise", "export", "pieces", "procedure"}
	negations := []string{"pas", "sans", "aucun", "aucune", "jamais", "ni"}

	for _, mot := range motsFatals {
		idx := strings.Index(titreBrut, mot)
		if idx != -1 {
			// Le mot fatal est trouvé. Est-il annulé par une négation juste avant ?
			estAnnule := false

			// On regarde les 20 caractères précédents pour trouver une négation
			start := idx - 20
			if start < 0 {
				start = 0
			}
			contexte := titreBrut[start:idx]

			for _, neg := range negations {
				if strings.Contains(contexte, neg) {
					estAnnule = true
					break
				}
			}

			// Si le mot fatal est là et PAS annulé -> On rejette
			if !estAnnule {
				return "", false
			}
		}
	}

	// Normalisation technique (Moteur)
	remplacements := map[string]string{
		"bluehdi": "hdi", "e-hdi": "hdi", "dci": "hdi", "tdi": "hdi", "crdi": "hdi",
		"puretech": "vti", "tce": "vti", "tfsi": "vti",
		"amg": "sport", "gti": "sport", "rs": "sport", "s line": "sport",
	}

	// Nettoyage regex
	reg := regexp.MustCompile("[^a-z0-9\\.]+")
	cleanTitre := reg.ReplaceAllString(titreBrut, " ")
	mots := strings.Fields(cleanTitre)

	var motsCles []string
	stopWords := []string{"vends", "superbe", "urgent", "propre", "top", "etat", "neuf", "parfait", "voiture"}

	for _, mot := range mots {
		if val, ok := remplacements[mot]; ok {
			mot = val
		}

		isStop := false
		for _, sw := range stopWords {
			if mot == sw {
				isStop = true
				break
			}
		}
		if !isStop && len(mot) > 1 {
			motsCles = append(motsCles, mot)
		}
	}

	if len(motsCles) < 2 {
		return "", false
	}

	max := 3
	if len(motsCles) < max {
		max = len(motsCles)
	}
	pattern := "%"
	for i := 0; i < max; i++ {
		pattern += motsCles[i] + "%"
	}

	return pattern, true
}

func getComparables(pattern string, annee, km int, carb string) []Comparable {
	// Fenêtre glissante : +/- 2 ans pour avoir des technologies comparables
	minAnnee, maxAnnee := annee-2, annee+2
	// Largeur KM : +/- 40% pour capter du volume avant filtrage
	minKm, maxKm := int(float64(km)*0.6), int(float64(km)*1.4)

	query := `
        SELECT prix, km, annee, date_creation FROM annonces 
        WHERE titre LIKE ? AND carburant = ? 
        AND annee BETWEEN ? AND ? AND km BETWEEN ? AND ?
        AND prix > 1500
        AND date_creation > datetime('now', '-180 days')
    `
	rows, err := database.DB.Query(query, pattern, carb, minAnnee, maxAnnee, minKm, maxKm)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var data []Comparable
	for rows.Next() {
		var c Comparable
		var d string
		if err := rows.Scan(&c.Prix, &c.Km, &c.Annee, &d); err == nil {
			if t, err := time.Parse("2006-01-02 15:04:05", d); err == nil {
				c.Date = t
			} else {
				c.Date = time.Now()
			}
			data = append(data, c)
		}
	}
	return data
}

// --- 2. STATISTIQUES & PONDÉRATION ---

func filtrerOutliers(data []Comparable) []Comparable {
	// Si on a peu de données, l'IQR est dangereux (risque de tout vider).
	// On ne l'applique que si on a > 8 véhicules.
	if len(data) <= 8 {
		return data
	}

	sort.Slice(data, func(i, j int) bool { return data[i].Prix < data[j].Prix })

	n := len(data)
	q1 := data[n/4].Prix
	q3 := data[(n*3)/4].Prix
	iqr := q3 - q1

	// Bornes de Tukey (1.5 * IQR)
	minVal := q1 - int(float64(iqr)*1.5)
	maxVal := q3 + int(float64(iqr)*1.5)

	var clean []Comparable
	for _, c := range data {
		if c.Prix >= minVal && c.Prix <= maxVal {
			clean = append(clean, c)
		}
	}
	return clean
}

func calculerMoyennesPonderees(data []Comparable, anneeCible, kmCible int) (int, int) {
	var sommePrix, sommeKm, totalPoids float64

	for _, c := range data {
		// Distance Euclidienne simplifiée
		diffKm := math.Abs(float64(c.Km - kmCible))
		diffAnnee := math.Abs(float64(c.Annee - anneeCible))

		// Pondération : 1 an d'écart pèse autant que 15000km d'écart
		scoreDistance := (diffKm / 15000.0) + diffAnnee

		// Poids inverse : plus c'est proche, plus ça compte
		poids := 1.0 / (1.0 + scoreDistance)

		sommePrix += float64(c.Prix) * poids
		sommeKm += float64(c.Km) * poids
		totalPoids += poids
	}

	if totalPoids == 0 {
		return 0, 0
	}
	return int(sommePrix / totalPoids), int(sommeKm / totalPoids)
}

func calculerTendance(data []Comparable) string {
	var sumRecent, sumOld float64
	var countRecent, countOld int
	now := time.Now()

	for _, c := range data {
		ageJours := now.Sub(c.Date).Hours() / 24
		if ageJours <= 30 {
			sumRecent += float64(c.Prix)
			countRecent++
		} else {
			sumOld += float64(c.Prix)
			countOld++
		}
	}
	if countRecent == 0 || countOld == 0 {
		return "Stable ➡️"
	}

	moyRecent := sumRecent / float64(countRecent)
	moyOld := sumOld / float64(countOld)
	diff := (moyRecent - moyOld) / moyOld * 100

	if diff > 5 {
		return "Hausse 📈"
	}
	if diff < -5 {
		return "Baisse 📉"
	}
	return "Stable ➡️"
}

// --- 3. MOTEUR D'AJUSTEMENT INDUSTRIEL ---

func ajusterPrixIndustriel(prixRef, kmRef, annee int, kmCible int, carb, tendance string) int {

	prix := float64(prixRef)

	// A. Tendance Marché
	if tendance == "Hausse 📈" {
		prix *= 1.05
	}
	if tendance == "Baisse 📉" {
		prix *= 0.95
	}

	// B. Correction KM Différentielle (Delta Km)
	deltaKm := kmRef - kmCible // Positif = Bonus (ma voiture a moins de km que la ref)

	// Courbe en S du coût kilométrique
	coutKm := 0.045 // Base
	if kmCible < 50000 {
		coutKm = 0.07 // Km "neuf"
	} else if kmCible > 160000 {
		coutKm = 0.03 // Km "routier"
	}

	bonusMalusKm := float64(deltaKm) * coutKm
	prix += bonusMalusKm

	// C. Décote Carburant Temporelle (Crit'Air / ZFE effect)
	age := time.Now().Year() - annee
	facteurCarb := 1.0

	if strings.Contains(strings.ToLower(carb), "diesel") {
		// Décote progressive après 8 ans (-1.5% par an supplémentaire)
		if age > 8 {
			anneesDifficiles := float64(age - 8)
			facteurCarb = 1.0 - (anneesDifficiles * 0.015)
		}
	} else if strings.Contains(strings.ToLower(carb), "hybride") {
		facteurCarb = 1.04 // Bonus Techno
	}

	// Sécurité plancher carburant (max -30%)
	if facteurCarb < 0.70 {
		facteurCarb = 0.70
	}

	prix *= facteurCarb

	// D. CEINTURE DE SÉCURITÉ FINALE (Bounding Global)
	// On empêche l'algo de partir en vrille mathématique
	// Le prix calculé ne doit pas s'éloigner de plus de 45% du prix de référence pondéré
	maxVariation := float64(prixRef) * 0.45
	minAllowed := float64(prixRef) - maxVariation
	maxAllowed := float64(prixRef) + maxVariation

	if prix < minAllowed {
		prix = minAllowed
	}
	if prix > maxAllowed {
		prix = maxAllowed
	}

	return int(prix)
}

func calculerConfiance(nb int, data []Comparable, prixEstime int) string {
	if nb < 5 {
		return "D"
	}
	var sumCarres float64
	moyenne := float64(prixEstime)
	for _, c := range data {
		d := float64(c.Prix) - moyenne
		sumCarres += d * d
	}
	ecartType := math.Sqrt(sumCarres / float64(nb))
	cv := ecartType / moyenne

	if nb > 20 && cv < 0.15 {
		return "A++"
	}
	if nb > 12 && cv < 0.20 {
		return "A"
	}
	if nb > 6 && cv < 0.30 {
		return "B"
	}
	return "C"
}

// --- PARTIE GEMINI ---
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}
type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}
type GeminiPart struct {
	Text string `json:"text"`
}
type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func demanderGemini(titre string, annee, km int, carb string, prix int) int {
	if len(config.GEMINI_API_KEY) < 10 {
		return 0
	}
	kmTxt := "km inconnu"
	if km > 0 {
		kmTxt = fmt.Sprintf("%d km", km)
	}
	prompt := fmt.Sprintf("Estime la cote revente France: %s, %d, %s, %s. Prix actuel: %d. Juste le chiffre.", titre, annee, kmTxt, carb, prix)

	reqBody, _ := json.Marshal(GeminiRequest{Contents: []GeminiContent{{Parts: []GeminiPart{{Text: prompt}}}}})
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-flash-latest:generateContent?key=" + config.GEMINI_API_KEY
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(reqBody))
	if err != nil || resp.StatusCode != 200 {
		return 0
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var gr GeminiResponse
	json.Unmarshal(body, &gr)
	if len(gr.Candidates) > 0 {
		reg := regexp.MustCompile("[^0-9]+")
		txt := reg.ReplaceAllString(gr.Candidates[0].Content.Parts[0].Text, "")
		var v int
		fmt.Sscanf(txt, "%d", &v)
		return v
	}
	return 0
}
