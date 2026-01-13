import discord
from discord.ext import commands, tasks
from discord.ui import Button, View
import sqlite3
import os
import json
import re
from datetime import datetime
import asyncio

# --- ⚙️ CONFIGURATION ---
TOKEN = "MTQ1OTY1MDA4NTE1ODY1NDA2Nw.G1wd9s.lT5ZvSjk5VK_BHqqRWRToNhUD23svqJ1yI37to"
ID_ROLE_MEMBRE = 1459646911911952395   # ID du rôle Membre (donné après règlement)
ID_ROLE_ADMIN = 1459646971848429568    # Ton ID de rôle (pour voir les tickets)
ID_CATEGORIE_TICKET = 1459652325311512616  # ID de la catégorie où créer les salons

ID_CATEGORIE_ALERTES = 1459892939001036820


# Chemins dynamiques
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
DB_PATH = os.path.join(BASE_DIR, '..', 'voitures.db') 
FICHIER_ALERTES = os.path.join(BASE_DIR, 'alerts.json')

intents = discord.Intents.default()
intents.members = True
intents.message_content = True
bot = commands.Bot(command_prefix="!", intents=intents, help_command=None) # On désactive l'aide par défaut

# --- GESTION DONNÉES ---
if os.path.exists(FICHIER_ALERTES):
    with open(FICHIER_ALERTES, "r") as f:
        try: active_alerts = json.load(f)
        except: active_alerts = {}
else:
    active_alerts = {}

def save_alerts():
    with open(FICHIER_ALERTES, "w") as f:
        json.dump(active_alerts, f, indent=4)

# --- VUES (BOUTONS) ---
class DeleteAlertView(View):
    def __init__(self, channel_id): 
        super().__init__(timeout=None)
        self.channel_id = str(channel_id)

    @discord.ui.button(label="🛑 Supprimer cette alerte", style=discord.ButtonStyle.danger, custom_id="btn_del_alert")
    async def delete_alert(self, interaction: discord.Interaction, button: Button):
        if self.channel_id in active_alerts:
            del active_alerts[self.channel_id]
            save_alerts()
        await interaction.response.send_message("🗑️ Alerte supprimée.", ephemeral=True)
        await asyncio.sleep(2)
        if interaction.channel: await interaction.channel.delete()

class TicketView(View):
    def __init__(self): super().__init__(timeout=None)
    @discord.ui.button(label="🎫 Ouvrir dossier d'achat", style=discord.ButtonStyle.blurple, emoji="💸", custom_id="btn_ticket_buy")
    async def create_ticket(self, interaction: discord.Interaction, button: Button):
        guild = interaction.guild
        cat = discord.utils.get(guild.categories, id=ID_CATEGORIE_TICKET)
        overwrites = {
            guild.default_role: discord.PermissionOverwrite(read_messages=False),
            interaction.user: discord.PermissionOverwrite(read_messages=True),
            guild.get_role(ID_ROLE_ADMIN): discord.PermissionOverwrite(read_messages=True)
        }
        chan = await guild.create_text_channel(f"achat-{interaction.user.name}", category=cat, overwrites=overwrites)
        await interaction.response.send_message(f"✅ Salon créé : {chan.mention}", ephemeral=True)
        await chan.send(f"👋 Salut {interaction.user.mention}! Un admin va venir.")

class ReglesView(View):
    def __init__(self): super().__init__(timeout=None)
    @discord.ui.button(label="✅ Accepter le règlement", style=discord.ButtonStyle.green, custom_id="btn_rules")
    async def accept(self, interaction: discord.Interaction, button: Button):
        role = interaction.guild.get_role(ID_ROLE_MEMBRE)
        if role:
            await interaction.user.add_roles(role)
            await interaction.response.send_message("✅ Bienvenue !", ephemeral=True)

# --- COMMANDES ---

@bot.command()
async def aide(ctx):
    """ Affiche le menu d'aide complet """
    embed = discord.Embed(title="📚 Guide du Chasseur Auto", description="Voici comment utiliser le bot pour traquer les meilleures affaires.", color=0xF1C40F)
    
    embed.add_field(name="🔍 Créer une alerte", value="`!alerte [Nom] [Filtres]`\nExemple simple : `!alerte Clio 4`", inline=False)
    
    filtres = "**`p:`** Prix Max (ex: `p:10000`)\n"
    filtres += "**`a:`** Année Min (ex: `a:2018`)\n"
    filtres += "**`k:`** Km Max (ex: `k:120000`)\n"
    filtres += "💡 *Pour le carburant, écris-le juste dans le nom (ex: `!alerte Golf 7 Diesel`)*"
    
    embed.add_field(name="⚙️ Les Filtres Disponibles", value=filtres, inline=False)
    
    ex_complet = "`!alerte Audi A3 p:15000 a:2016 k:140000`"
    embed.add_field(name="🏆 Exemple Pro", value=ex_complet, inline=False)
    
    embed.add_field(name="📋 Gérer ses alertes", value="`!mes_alertes` : Affiche ton tableau de bord.", inline=False)
    
    await ctx.send(embed=embed)

@bot.command()
async def mes_alertes(ctx):
    """ Dashboard utilisateur """
    user_alerts = []
    ids_to_clean = []

    for channel_id, data in active_alerts.items():
        if data["user_id"] == ctx.author.id:
            channel = bot.get_channel(int(channel_id))
            if channel: user_alerts.append((channel, data))
            else: ids_to_clean.append(channel_id)
    
    for cid in ids_to_clean: del active_alerts[cid]
    if ids_to_clean: save_alerts()

    if not user_alerts:
        await ctx.send("❌ Tu n'as aucune alerte. Tape `!aide` pour commencer !")
        return

    embed = discord.Embed(title="📋 Tes Alertes Actives", color=0x3498db)
    for channel, data in user_alerts:
        f = []
        if data['max_price']: f.append(f"💰 < {data['max_price']}€")
        if data['min_year']: f.append(f"📅 > {data['min_year']}")
        if data['max_km']: f.append(f"🛣️ < {data['max_km']}km")
        
        f_str = " | ".join(f) if f else "Aucun filtre"
        embed.add_field(name=f"🔎 {data['keyword'].title()}", value=f"{f_str}\n👉 {channel.mention}", inline=False)

    await ctx.send(embed=embed)

@bot.command()
async def alerte(ctx, *, args: str):
    """ Création d'alerte avec scan rétroactif intelligent """
    prix_max = None
    annee_min = None
    km_max = None
    
    # 1. Parsing des filtres (Regex)
    # Prix p:10000
    if m := re.search(r'p:(\d+)', args):
        prix_max = int(m.group(1))
        args = args.replace(m.group(0), "")
    # Année a:2015
    if m := re.search(r'a:(\d+)', args):
        annee_min = int(m.group(1))
        args = args.replace(m.group(0), "")
    # Km k:150000
    if m := re.search(r'k:(\d+)', args):
        km_max = int(m.group(1))
        args = args.replace(m.group(0), "")

    mot_cle = args.strip()
    if len(mot_cle) < 2:
        await ctx.send("❌ Il manque le nom de la voiture ! (Ex: `!alerte Clio`)")
        return

    # 2. Création Salon
    guild = ctx.guild
    cat = discord.utils.get(guild.categories, id=ID_CATEGORIE_ALERTES)
    nom_salon = f"🔎-{mot_cle[:8]}-{ctx.author.name}".lower().replace(" ", "-")
    
    overwrites = {
        guild.default_role: discord.PermissionOverwrite(read_messages=False),
        ctx.author: discord.PermissionOverwrite(read_messages=True),
        guild.get_role(ID_ROLE_ADMIN): discord.PermissionOverwrite(read_messages=True)
    }

    try: channel = await guild.create_text_channel(nom_salon, category=cat, overwrites=overwrites)
    except Exception as e:
        await ctx.send(f"❌ Erreur: {e}")
        return

    # 3. Sauvegarde
    active_alerts[str(channel.id)] = {
        "user_id": ctx.author.id,
        "keyword": mot_cle.lower(),
        "max_price": prix_max,
        "min_year": annee_min,
        "max_km": km_max,
        "last_scan_time": datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    }
    save_alerts()

    # 4. Message Bienvenue
    desc = f"**Recherche :** {mot_cle.title()}"
    if prix_max: desc += f"\n💰 **Prix Max :** {prix_max} €"
    if annee_min: desc += f"\n📅 **Année Min :** {annee_min}"
    if km_max: desc += f"\n🛣️ **Km Max :** {km_max} km"

    embed = discord.Embed(title="✅ Alerte Activée", description=desc, color=0x2ecc71)
    embed.set_footer(text="Je scanne l'historique et les nouvelles annonces...")
    await channel.send(content=f"{ctx.author.mention}", embed=embed, view=DeleteAlertView(channel.id))
    
    # Confirmation dans le salon commandes
    await ctx.send(f"✅ Salon créé : {channel.mention}", delete_after=5)
    try: await ctx.message.delete()
    except: pass

    # 5. SCAN RÉTROACTIF (Historique 24h)
    if os.path.exists(DB_PATH):
        try:
            conn = sqlite3.connect(f'file:{DB_PATH}?mode=ro', uri=True)
            cursor = conn.cursor()
            
            # Construction Query dynamique
            query = "SELECT titre, prix, annee, km, carburant, ville, estimation_ia, url, image FROM annonces WHERE date_creation >= datetime('now', '-1 day')"
            params = []
            
            query += " AND lower(titre) LIKE ?"
            params.append(f"%{mot_cle.lower()}%")

            if prix_max:
                query += " AND prix <= ?"
                params.append(prix_max)
            if annee_min:
                query += " AND annee >= ?"
                params.append(annee_min)
            if km_max:
                query += " AND km <= ?"
                params.append(km_max)
            
            query += " ORDER BY date_creation DESC LIMIT 5" # Max 5 résultats historiques
            
            cursor.execute(query, params)
            results = cursor.fetchall()
            conn.close()

            if results:
                await channel.send("👀 **Trouvé dans les dernières 24h :**")
                for car in results:
                    # Fonction d'affichage commune
                    await send_car_embed(channel, car)
            else:
                await channel.send("🚫 Rien dans les dernières 24h. Je guette les nouvelles !")

        except Exception as e:
            print(f"Erreur Retro-Scan: {e}")

# --- FONCTION D'AFFICHAGE COMMUNE (Pour éviter de répéter le code) ---
async def send_car_embed(channel, car_data):
    titre, prix, annee, km, carb, ville, cote, url, image = car_data[:9] # On prend les 9 premiers champs

    # Calcul Rentabilité
    analyse = ""
    color = 0x3498db # Bleu neutre
    
    if cote > 0:
        marge = ((cote - prix) / cote) * 100
        if marge > 20: 
            analyse = f"\n🔥 **Super Deal !** (Cote: {cote}€ | Marge: +{int(marge)}%)"
            color = 0xe74c3c # Rouge
        elif marge > 5:
            analyse = f"\n✅ **Bonne Affaire** (Cote: {cote}€)"
            color = 0x2ecc71 # Vert
        elif marge < -10:
             analyse = f"\n❌ **Trop cher** (Cote: {cote}€)"
             color = 0x95a5a6 # Gris
    
    # Construction Embed
    embed = discord.Embed(title=f"🎯 {titre}", url=url, color=color)
    embed.add_field(name="Prix", value=f"**{prix} €**", inline=True)
    embed.add_field(name="Année", value=f"{annee}", inline=True)
    embed.add_field(name="Km", value=f"{km} km", inline=True)
    embed.add_field(name="Infos", value=f"⛽ {carb}\n📍 {ville}{analyse}", inline=False)
    
    if image: embed.set_image(url=image)
    
    view = View()
    view.add_item(Button(label="Voir l'annonce", style=discord.ButtonStyle.link, url=url))
    
    await channel.send(embed=embed, view=view)


# --- BOUCLE PRINCIPALE (SNIPER) ---
@tasks.loop(seconds=10)
async def check_alerts():
    if not active_alerts: return
    if not os.path.exists(DB_PATH): return

    try:
        conn = sqlite3.connect(f'file:{DB_PATH}?mode=ro', uri=True)
        cursor = conn.cursor()
        # On ne charge que les annonces très fraîches (dernières 10 min pour être sûr de ne rien rater)
        cursor.execute("SELECT titre, prix, annee, km, carburant, ville, estimation_ia, url, image, date_creation FROM annonces WHERE date_creation >= datetime('now', '-10 minutes') ORDER BY date_creation DESC")
        recent_cars = cursor.fetchall()
        conn.close()

        if not recent_cars: return
        
        ids_to_clean = []

        for channel_id, data in active_alerts.items():
            channel = bot.get_channel(int(channel_id))
            if not channel:
                ids_to_clean.append(channel_id)
                continue

            # Récupération filtres
            keyword = data["keyword"]
            max_p = data.get("max_price")
            min_y = data.get("min_year")
            max_k = data.get("max_km")
            
            try: last_scan = datetime.strptime(data["last_scan_time"], "%Y-%m-%d %H:%M:%S")
            except: last_scan = datetime.now()
            new_last_scan = last_scan
            
            for car in recent_cars:
                # car contient 10 éléments, le dernier est la date
                date_str = car[9]
                try: date_creation = datetime.strptime(date_str, "%Y-%m-%d %H:%M:%S")
                except: continue

                # On vérifie si c'est nouveau POUR CET UTILISATEUR
                if date_creation > last_scan:
                    titre = car[0]
                    prix = car[1]
                    annee = car[2]
                    km = car[3]

                    # Filtres
                    if keyword not in titre.lower(): continue
                    if max_p is not None and prix > max_p: continue
                    if min_y is not None and annee < min_y: continue
                    if max_k is not None and km > max_k: continue

                    # BINGO -> Envoi
                    await channel.send("🔔 **Nouvelle annonce détectée !**")
                    await send_car_embed(channel, car)
                    
                    if date_creation > new_last_scan:
                        new_last_scan = date_creation

            active_alerts[channel_id]["last_scan_time"] = new_last_scan.strftime("%Y-%m-%d %H:%M:%S")
        
        for cid in ids_to_clean: del active_alerts[cid]
        if ids_to_clean or recent_cars: save_alerts()

    except Exception as e:
        print(f"Erreur Loop: {e}")

@bot.event
async def on_ready():
    bot.add_view(ReglesView())
    bot.add_view(TicketView())
    for cid in active_alerts: bot.add_view(DeleteAlertView(cid))
    check_alerts.start()
    print(f"🐍 Bot Python V26 Connecté : {bot.user}")

@bot.command()
async def setup_ticket(ctx):
    await ctx.message.delete()
    await ctx.send("💎 **OUVRIR UN DOSSIER D'ACHAT**", view=TicketView())

@bot.command()
async def setup_regles(ctx):
    await ctx.message.delete()
    await ctx.send("📜 **RÈGLEMENT**", view=ReglesView())

bot.run(TOKEN)