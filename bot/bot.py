import discord
from discord import app_commands
from discord.ext import commands, tasks
import aiosqlite
import os
import asyncio
import re
from datetime import datetime, timedelta
from dataclasses import dataclass
from typing import Optional
from dotenv import load_dotenv

# ==========================================
# ⚙️ CONFIGURATION
# ==========================================

# ⚠️ REMPLACE CECI PAR UN FICHIER .ENV EN PROD
load_dotenv()
TOKEN = os.getenv("TOKEN")

DB_NAME = "voitures.db"

def get_id(name):
    val = os.getenv(name)
    if not val:
        print(f"⚠️ ATTENTION : La variable {name} est vide dans le fichier .env")
        return 0
    return int(val)
# IDs (À vérifier avec tes propres IDs Discord)


ID_ROLE_MEMBRE = get_id("ID_ROLE_MEMBRE")   #id salon pour les membres 
ID_ROLE_ADMIN = get_id("ID_ROLE_ADMIN")   #id salon pour les admin
ID_ROLE_VIP = get_id("ID_ROLE_VIP")  #id salon pour les vips
ID_CATEGORIE_TICKET = get_id("ID_CATEGORIE_TICKET") #id du salon pour les tickets
ID_CATEGORIE_ALERTES = get_id("ID_CATEGORIE_ALERTES") #id du salon pour les alertes
ID_SALON_TARIF = get_id("ID_SALON_TARIF") #id_salon tarif 
ID_SALON_GENERAL = get_id("ID_SALON_GENERAL") #id_salon general 
ID_SALON_CCM = get_id("ID_SALON_CCM") #id_salon comment ça marche
ID_SALON_CMD = get_id("ID_SALON_CMD") #id salon commande

# ==========================================
# 🧠 CLASSES DE DONNÉES
# ==========================================

@dataclass
class CarOffer:
    id: int
    titre: str
    prix: int
    annee: int
    km: int
    carburant: str
    ville: str
    cote: int
    url: str
    image_url: str
    date_creation: str

    @property
    def marge(self) -> float:
        if self.cote <= 0: return 0.0
        return ((self.cote - self.prix) / self.cote) * 100

# ==========================================
# 🗄️ GESTIONNAIRE BASE DE DONNÉES (ASYNC)
# ==========================================

class DatabaseManager:
    def __init__(self, db_name):
        self.db_name = db_name

    async def init_db(self):
        async with aiosqlite.connect(self.db_name) as db:
            # Table Annonces
            await db.execute("""
                CREATE TABLE IF NOT EXISTS annonces (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    titre TEXT, prix INTEGER, annee INTEGER, km INTEGER,
                    carburant TEXT, ville TEXT, estimation_ia INTEGER,
                    url TEXT UNIQUE, image TEXT, date_creation TIMESTAMP
                )
            """)
            # Table Alertes
            await db.execute("""
                CREATE TABLE IF NOT EXISTS alertes (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    user_id INTEGER,
                    channel_id INTEGER,
                    keyword TEXT,
                    max_price INTEGER,
                    min_year INTEGER,
                    max_km INTEGER,
                    ville TEXT,
                    date_creation TIMESTAMP
                )
            """)
            # Table VIPs (Remplace vips.json)
            await db.execute("""
                CREATE TABLE IF NOT EXISTS vips (
                    user_id INTEGER PRIMARY KEY,
                    date_fin TIMESTAMP
                )
            """)
            await db.commit()

    # --- ALERTES ---
    async def add_alert(self, user_id, channel_id, keyword, max_price, min_year, max_km, ville):
        async with aiosqlite.connect(self.db_name) as db:
            await db.execute("""
                INSERT INTO alertes (user_id, channel_id, keyword, max_price, min_year, max_km, ville, date_creation)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            """, (user_id, channel_id, keyword, max_price, min_year, max_km, ville, datetime.now()))
            await db.commit()

    async def get_active_alerts(self):
        async with aiosqlite.connect(self.db_name) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute("SELECT * FROM alertes") as cursor:
                return await cursor.fetchall()
    
    async def get_user_alerts(self, user_id):
        async with aiosqlite.connect(self.db_name) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute("SELECT * FROM alertes WHERE user_id = ?", (user_id,)) as cursor:
                return await cursor.fetchall()

    async def delete_alert(self, channel_id):
        async with aiosqlite.connect(self.db_name) as db:
            await db.execute("DELETE FROM alertes WHERE channel_id = ?", (channel_id,))
            await db.commit()

    # --- VIPS ---
    async def add_vip(self, user_id, date_fin):
        async with aiosqlite.connect(self.db_name) as db:
            # REPLACE INTO permet d'écraser l'ancien abonnement si le membre reprend un VIP
            await db.execute("REPLACE INTO vips (user_id, date_fin) VALUES (?, ?)", (user_id, date_fin))
            await db.commit()

    async def get_expired_vips(self):
        now = datetime.now()
        async with aiosqlite.connect(self.db_name) as db:
            db.row_factory = aiosqlite.Row
            async with db.execute("SELECT * FROM vips WHERE date_fin < ?", (now,)) as cursor:
                return await cursor.fetchall()

    async def remove_vip(self, user_id):
        async with aiosqlite.connect(self.db_name) as db:
            await db.execute("DELETE FROM vips WHERE user_id = ?", (user_id,))
            await db.commit()

    # --- ANNONCES ---
    async def get_recent_cars(self, since_date):
        async with aiosqlite.connect(self.db_name) as db:
            db.row_factory = aiosqlite.Row
            query = "SELECT * FROM annonces WHERE date_creation > ?"
            async with db.execute(query, (since_date,)) as cursor:
                rows = await cursor.fetchall()
                cars = []
                for row in rows:
                    try:
                        cars.append(CarOffer(
                            id=row['id'], titre=row['titre'], prix=row['prix'], 
                            annee=row['annee'], km=row['km'], carburant=row['carburant'], 
                            ville=row['ville'], cote=row['estimation_ia'],
                            url=row['url'], image_url=row['image'], date_creation=row['date_creation']
                        ))
                    except: continue
                return cars

db_manager = DatabaseManager(DB_NAME)

# ==========================================
# 🖥️ VUES (BOUTONS INTERACTIFS)
# ==========================================

class DeleteAlertView(discord.ui.View):
    def __init__(self, channel_id):
        super().__init__(timeout=None)
        self.channel_id = channel_id

    @discord.ui.button(label="🛑 Arrêter et Supprimer", style=discord.ButtonStyle.danger, custom_id="stop_alert")
    async def stop(self, interaction: discord.Interaction, button: discord.ui.Button):
        await interaction.response.defer()
        await db_manager.delete_alert(self.channel_id)
        if interaction.channel: await interaction.channel.delete()

class TicketView(discord.ui.View):
    def __init__(self): super().__init__(timeout=None)
    @discord.ui.button(label="🎫 Ouvrir dossier d'achat", style=discord.ButtonStyle.blurple, emoji="💸", custom_id="btn_ticket_buy")
    async def create_ticket(self, interaction: discord.Interaction, button: discord.ui.Button):
        guild = interaction.guild
        cat = discord.utils.get(guild.categories, id=ID_CATEGORIE_TICKET)
        if not cat: return await interaction.response.send_message("❌ Erreur Config Ticket.", ephemeral=True)

        overwrites = {
            guild.default_role: discord.PermissionOverwrite(view_channel=False),
            interaction.user: discord.PermissionOverwrite(view_channel=True, send_messages=True),
            guild.get_role(ID_ROLE_ADMIN): discord.PermissionOverwrite(view_channel=True, send_messages=True)
        }
        nom_salon = f"achat-{interaction.user.name}"
        try:
            chan = await guild.create_text_channel(nom_salon, category=cat, overwrites=overwrites)
            await interaction.response.send_message(f"✅ Dossier : {chan.mention}", ephemeral=True)
            await chan.send(f"👋 {interaction.user.mention}, un admin va s'occuper de toi.")
        except Exception as e:
            await interaction.response.send_message(f"❌ Erreur: {e}", ephemeral=True)

class ReglesView(discord.ui.View):
    def __init__(self): super().__init__(timeout=None)
    @discord.ui.button(label="✅ Accepter le règlement", style=discord.ButtonStyle.green, custom_id="btn_rules")
    async def accept(self, interaction: discord.Interaction, button: discord.ui.Button):
        role = interaction.guild.get_role(ID_ROLE_MEMBRE)
        if role in interaction.user.roles:
            await interaction.response.send_message("Déjà validé !", ephemeral=True)
        else:
            await interaction.user.add_roles(role)
            await interaction.response.send_message(f"🎉 Bienvenue {interaction.user.mention} !", ephemeral=True)

# ==========================================
# 🤖 LE BOT (CLIENT)
# ==========================================

class SniperBot(commands.Bot):
    def __init__(self):
        super().__init__(command_prefix="!", intents=discord.Intents.all(), help_command=None)
        self.last_scan = datetime.now()

    async def setup_hook(self):
        await db_manager.init_db()
        # On ajoute les vues persistantes pour qu'elles marchent après reboot
        self.add_view(TicketView())
        self.add_view(ReglesView())
        await self.tree.sync()
        print("✅ Système prêt.")
        self.scanner_task.start()
        self.check_vip_expiration.start()

    @tasks.loop(seconds=15)
    async def scanner_task(self):
        try:
            nouvelles_annonces = await db_manager.get_recent_cars(self.last_scan)
            if nouvelles_annonces:
                print(f"🔍 {len(nouvelles_annonces)} nouvelles annonces.")
                active_alerts = await db_manager.get_active_alerts()
                for voiture in nouvelles_annonces:
                    await self.dispatch_alert(voiture, active_alerts)
                self.last_scan = datetime.now()
        except Exception as e:
            print(f"❌ Erreur Scan: {e}")

    @tasks.loop(minutes=60)
    async def check_vip_expiration(self):
        """ Vérifie les VIPs expirés toutes les heures """
        try:
            expired_vips = await db_manager.get_expired_vips()
            if not expired_vips: return
            
            guild = self.get_guild(self.guilds[0].id) if self.guilds else None
            if not guild: return
            
            role_vip = guild.get_role(ID_ROLE_VIP)
            
            for row in expired_vips:
                user_id = row['user_id']
                member = guild.get_member(user_id)
                if member and role_vip:
                    await member.remove_roles(role_vip)
                    print(f"📉 VIP expiré retiré pour {member.name}")
                
                # On retire de la DB
                await db_manager.remove_vip(user_id)
        except Exception as e:
            print(f"❌ Erreur VIP Check: {e}")

    async def dispatch_alert(self, voiture: CarOffer, alerts):
        for alert in alerts:
            if alert['keyword'] and alert['keyword'].lower() not in voiture.titre.lower(): continue
            if alert['max_price'] and voiture.prix > alert['max_price']: continue
            if alert['min_year'] and voiture.annee < alert['min_year']: continue
            if alert['max_km'] and voiture.km > alert['max_km']: continue
            
            if alert['ville']:
                v_car = voiture.ville.lower() if voiture.ville else ""
                v_alert = alert['ville'].lower()
                if v_alert.isdigit() and len(v_alert) <= 3:
                    match = re.search(r'\b(\d{5})\b', v_car)
                    if match:
                        if not match.group(1).startswith(v_alert): continue
                    else:
                        if v_alert not in v_car: continue
                elif v_alert not in v_car: continue

            channel = self.get_channel(alert['channel_id'])
            if channel:
                asyncio.create_task(self.safe_send(channel, voiture))
            else:
                asyncio.create_task(db_manager.delete_alert(alert['channel_id']))

    async def safe_send(self, channel, voiture: CarOffer):
        try:
            await self.send_car_embed(channel, voiture)
            await asyncio.sleep(0.5) 
        except (discord.Forbidden, discord.NotFound):
            await db_manager.delete_alert(channel.id)

    async def send_car_embed(self, channel, voiture: CarOffer):
        color = 0x3498db
        if voiture.marge > 20: color = 0x2ecc71
        if voiture.marge > 30: color = 0xf1c40f
        
        embed = discord.Embed(title=f"🚗 {voiture.titre}", url=voiture.url, color=color)
        embed.add_field(name="Infos", value=f"**{voiture.prix} €** • {voiture.annee} • {voiture.km} km", inline=False)
        embed.add_field(name="Lieu", value=voiture.ville or "Non spécifié", inline=True)
        if voiture.cote > 0:
            embed.add_field(name="Rentabilité", value=f"📉 Cote: {voiture.cote}€\n💰 Marge: **{int(voiture.marge)}%**", inline=True)
        if voiture.image_url: embed.set_image(url=voiture.image_url)
        view = discord.ui.View()
        view.add_item(discord.ui.Button(label="Voir l'annonce", url=voiture.url))
        await channel.send(embed=embed, view=view)

bot = SniperBot()

# ==========================================
# 🎮 COMMANDES (ANCIENNES + SLASH)
# ==========================================

# 1. ALERTES (Slash Command - Moderne)
@bot.tree.command(name="alerte", description="Créer une alerte personnalisée")
@app_commands.describe(mots_cles="Ex: Clio 4 RS", prix_max="Prix Max €", ville="Ville ou Dept (69)")
async def alerte(interaction: discord.Interaction, mots_cles: str, prix_max: Optional[int], annee_min: Optional[int], km_max: Optional[int], ville: Optional[str]):
    guild = interaction.guild
    categorie = discord.utils.get(guild.categories, id=ID_CATEGORIE_ALERTES)
    if not categorie: return await interaction.response.send_message("❌ Erreur Catégorie.", ephemeral=True)

    safe_name = f"recherche-{interaction.user.name}"[:25].lower().replace(" ", "-")
    overwrites = {
        guild.default_role: discord.PermissionOverwrite(read_messages=False),
        interaction.user: discord.PermissionOverwrite(read_messages=True),
        guild.get_role(ID_ROLE_ADMIN): discord.PermissionOverwrite(read_messages=True)
    }

    try:
        channel = await guild.create_text_channel(name=safe_name, category=categorie, overwrites=overwrites)
        await db_manager.add_alert(interaction.user.id, channel.id, mots_cles, prix_max, annee_min, km_max, ville)
        await interaction.response.send_message(f"✅ Salon créé : {channel.mention}", ephemeral=True)
        await channel.send(f"👋 Bienvenue {interaction.user.mention} ! Scan en cours...", view=DeleteAlertView(channel.id))
    except Exception as e:
        await interaction.response.send_message(f"❌ Erreur: {e}", ephemeral=True)

# 2. MES ALERTES (Ancienne commande remise)
@bot.command()
async def mes_alertes(ctx):
    """ Affiche vos alertes actives """
    if ctx.channel.id != ID_SALON_CMD: return
    
    alerts = await db_manager.get_user_alerts(ctx.author.id)
    if not alerts: return await ctx.send("❌ Aucune alerte active.")
    
    msg = "**📋 Tes Alertes :**\n"
    for row in alerts:
        msg += f"• {row['keyword']} (<#{row['channel_id']}>)\n"
    await ctx.send(msg)

# 3. AIDE & TARIF & SETUP (Anciennes commandes remises)
@bot.command()
async def aide(ctx):
    if ctx.channel.id not in [ID_SALON_CCM, ID_SALON_GENERAL, ID_SALON_CMD]: return
    embed = discord.Embed(title="🕵️ Guide du Chasseur", color=0x00ff00)
    embed.add_field(name="🚀 Créer une alerte", value="Utilise la commande `/alerte`", inline=False)
    await ctx.send(embed=embed)

@bot.command()
async def tarif(ctx):
    if ID_SALON_TARIF and ctx.channel.id != ID_SALON_TARIF: return
    embed = discord.Embed(title="💎 Devenez Chasseur d'Élite", description="Passez à la vitesse supérieure.", color=0xF1C40F)
    embed.add_field(name="🚀 Membre VIP", value="Alertes Instantanées • Cote & Marge • Multi-Sources", inline=False)
    embed.add_field(name="💸 Tarif", value="**29.99€ / mois**", inline=False)
    await ctx.send(embed=embed, view=TicketView())

@bot.command()
@commands.has_permissions(administrator=True)
async def setup_ticket(ctx):
    await ctx.message.delete()
    await ctx.send("💎 **OUVRIR UN DOSSIER**", view=TicketView())

@bot.command()
@commands.has_permissions(administrator=True)
async def setup_regles(ctx):
    await ctx.message.delete()
    embed = discord.Embed(title="📜 RÈGLEMENT", description="Respect • Pas de spam • Responsabilité", color=0x2ecc71)
    await ctx.send(embed=embed, view=ReglesView())

# 4. ADMINISTRATION (CLEAN, FERMER, VIP)
@bot.command()
@commands.has_permissions(manage_messages=True)
async def clean(ctx, limit: int = None):
    try: await ctx.message.delete()
    except: pass
    await ctx.channel.purge(limit=limit)

@bot.command()
@commands.has_permissions(manage_channels=True)
async def fermer(ctx):
    await ctx.send("🔒 Fermeture...")
    await asyncio.sleep(2)
    await ctx.channel.delete()

@bot.command()
@commands.has_permissions(administrator=True)
async def vip(ctx, membre: discord.Member, duree: str):
    """ !vip @User 1m (Donne le VIP pour 1 mois) """
    now = datetime.now()
    if duree == "1s": date_fin = now + timedelta(weeks=1)
    elif duree == "1m": date_fin = now + timedelta(days=30)
    else: return await ctx.send("❌ Usage: `!vip @User 1s` ou `1m`")

    role_vip = ctx.guild.get_role(ID_ROLE_VIP)
    if not role_vip: return await ctx.send("❌ Erreur Config ID Rôle VIP")

    try: 
        await membre.add_roles(role_vip)
        # SAUVEGARDE EN BDD (Plus de JSON !)
        await db_manager.add_vip(membre.id, date_fin)
        await ctx.send(f"💎 VIP activé pour {membre.mention} jusqu'au {date_fin.strftime('%d/%m/%Y')}")
    except Exception as e: 
        await ctx.send(f"❌ Erreur: {e}")

# ==========================================
# 🚀 DÉMARRAGE
# ==========================================

@bot.event
async def on_ready():
    print(f"Connecté en tant que {bot.user}")

if __name__ == "__main__":
    bot.run(TOKEN)