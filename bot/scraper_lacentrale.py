import asyncio
from playwright.async_api import async_playwright
import sqlite3
import os
import random
import re
from datetime import datetime

# Chemin vers la DB (même que le bot discord)
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
DB_PATH = os.path.join(BASE_DIR, '..', 'voitures.db')

# Fonction pour calculer la cote (version simplifiée Python)
def estimer_prix(titre, annee, km):
    base = 0
    t = titre.lower()
    if "clio" in t: base = 13000 if annee >= 2019 else 7000
    elif "208" in t: base = 14000 if annee >= 2019 else 7000
    elif "golf" in t: base = 23000 if annee >= 2020 else 12000
    
    if base == 0: return 0
    
    age = datetime.now().year - annee
    if age < 1: age = 1
    km_std = age * 15000
    
    if km > 0:
        base = base - int((km - km_std) * 0.04)
    return max(base, 1000)

async def run():
    print("🔵 Démarrage Scraper LaCentrale (Mode Navigateur Réel)...")
    
    async with async_playwright() as p:
        # On lance un vrai navigateur Chrome (headless=True pour le cacher, False pour le voir travailler)
        browser = await p.chromium.launch(headless=False) 
        context = await browser.new_context(
            user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            viewport={"width": 1280, "height": 720}
        )
        
        # On ajoute un script pour masquer que c'est un bot
        await context.add_init_script("Object.defineProperty(navigator, 'webdriver', {get: () => undefined})")

        page = await context.new_page()

        while True:
            try:
                print("\n🔵 Chargement LaCentrale...")
                await page.goto("https://www.lacentrale.fr/listing?sorting=CREATION_DATE_DESC", timeout=60000)
                
                # On attend que les annonces chargent (on cherche le prix)
                try:
                    await page.wait_for_selector(".searchCard__price", timeout=10000)
                except:
                    print("⚠️  Pas d'annonces ou Blocage détecté. Pause...")
                    await asyncio.sleep(10)
                    continue

                # Récupération des données
                annonces = await page.locator("a[href^='/auto-occasion-annonce']").all()
                print(f"   🔎 {len(annonces)} annonces détectées sur la page.")

                conn = sqlite3.connect(DB_PATH)
                cursor = conn.cursor()
                count_new = 0

                for annonce in annonces:
                    try:
                        url_part = await annonce.get_attribute("href")
                        url = "https://www.lacentrale.fr" + url_part
                        id_annonce = url_part.split("-")[-1].replace(".html", "")

                        # Vérif DB
                        cursor.execute("SELECT 1 FROM annonces WHERE id = ? AND site = 'LaCentrale'", (id_annonce,))
                        if cursor.fetchone(): continue

                        text = await annonce.inner_text()
                        
                        # Extraction bourrin (similaire au Go)
                        titre = "Voiture Inconnue"
                        lignes = text.split('\n')
                        if len(lignes) > 0: titre = lignes[0]

                        # Prix
                        prix = 0
                        prix_match = re.search(r'(\d[\d\s]+)€', text)
                        if prix_match: prix = int(prix_match.group(1).replace(" ", ""))

                        # Année
                        annee = 0
                        annee_match = re.search(r'\b(20[0-2][0-9])\b', text)
                        if annee_match: annee = int(annee_match.group(1))

                        # Km
                        km = 0
                        km_match = re.search(r'(\d[\d\s]+)km', text)
                        if km_match: km = int(km_match.group(1).replace(" ", ""))

                        # Carburant
                        carb = "Essence"
                        if "Diesel" in text: carb = "Diesel"
                        elif "Hybride" in text: carb = "Hybride"
                        elif "Électrique" in text: carb = "Électrique"

                        img = await annonce.locator("img").first.get_attribute("src")
                        if not img: img = ""

                        if prix > 500 and annee > 2000:
                            cote = estimer_prix(titre, annee, km)
                            
                            cursor.execute("""
                                INSERT INTO annonces (id, site, titre, prix, annee, km, carburant, ville, estimation_ia, url, image, envoye_gratuit)
                                VALUES (?, 'LaCentrale', ?, ?, ?, ?, ?, 'France', ?, ?, ?, 0)
                            """, (id_annonce, titre, prix, annee, km, carb, cote, url, img))
                            conn.commit()
                            print(f"✅ LC: {titre} | {prix}€")
                            count_new += 1

                    except Exception as e:
                        continue # On passe à l'annonce suivante si erreur

                conn.close()
                if count_new > 0: print(f"💾 {count_new} nouvelles annonces sauvegardées.")

                # Pause humaine aléatoire
                pause = random.randint(40, 80)
                print(f"💤 Pause {pause}s...")
                await asyncio.sleep(pause)

            except Exception as e:
                print(f"❌ Erreur Globale: {e}")
                await asyncio.sleep(30)

if __name__ == "__main__":
    asyncio.run(run())