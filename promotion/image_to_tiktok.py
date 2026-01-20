from moviepy.editor import *
import os
import numpy as np
from PIL import Image, ImageFilter, ImageDraw, ImageFont

# --- CONFIGURATION ---
IMAGE_PATH = "1.jpg"       # Ton image (renomme ta capture ainsi)
MUSIC_PATH = "music_test.mp3"    # Ta musique
OUTPUT_PATH = "tiktok_final.mp4"
DURATION = 6                     # Durée courte = Meilleur taux de répétition (loop)

print("🎬 Transformation pour TikTok en cours...")

# --- FONCTION TEXTE (Pour le "LIEN EN BIO") ---
def create_cta_text(text, fontsize, color, bg_color):
    # Charge une police
    try: font = ImageFont.truetype("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", fontsize)
    except: font = ImageFont.load_default()

    # Taille
    dummy = ImageDraw.Draw(Image.new('RGB', (1, 1)))
    bbox = dummy.textbbox((0, 0), text, font=font)
    w, h = bbox[2] - bbox[0], bbox[3] - bbox[1]
    
    # Image
    img = Image.new('RGBA', (w + 40, h + 40), bg_color)
    draw = ImageDraw.Draw(img)
    draw.text((20, 20), text, font=font, fill=color)
    return np.array(img)

try:
    # 1. PRÉPARATION DE L'IMAGE
    pil_img = Image.open(IMAGE_PATH).convert("RGBA")
    
    # --- A. LE FOND (Background) ---
    # On veut remplir 1920px de haut
    ratio = 1920 / pil_img.height
    new_w = int(pil_img.width * ratio)
    new_h = 1920
    
    # Redimensionnement et Flou
    bg_pil = pil_img.resize((new_w, new_h), Image.LANCZOS)
    bg_pil = bg_pil.filter(ImageFilter.GaussianBlur(radius=25)) # Gros flou
    
    # Assombrir (Calque noir à 60% d'opacité)
    overlay = Image.new('RGBA', bg_pil.size, (0, 0, 0, 150))
    bg_pil = Image.alpha_composite(bg_pil, overlay)
    
    bg_clip = ImageClip(np.array(bg_pil)).set_duration(DURATION).set_position("center")

    # --- B. L'IMAGE CENTRALE (Foreground) ---
    # On veut qu'elle prenne la largeur de l'écran (1080) moins une marge
    target_width = 950 # Marge de sécurité
    ratio_fg = target_width / pil_img.width
    fg_h = int(pil_img.height * ratio_fg)
    
    fg_pil = pil_img.resize((target_width, fg_h), Image.LANCZOS)
    fg_clip = ImageClip(np.array(fg_pil)).set_duration(DURATION).set_position("center")

    # --- C. APPEL À L'ACTION (CTA) ---
    # Création d'un bouton "LIEN EN BIO 🔗"
    cta_arr = create_cta_text("LIEN EN BIO 🔗", 50, "white", (255, 0, 0, 255)) # Fond rouge
    cta_clip = ImageClip(cta_arr).set_duration(DURATION).set_position(("center", 1600)) # En bas

    # --- 2. MONTAGE ---
    # On crop le fond pour être sûr qu'il fait 1080x1920
    final = CompositeVideoClip([bg_clip, fg_clip, cta_clip], size=(1080, 1920))

    # --- 3. AUDIO ---
    if os.path.exists(MUSIC_PATH):
        audio = AudioFileClip(MUSIC_PATH)
        if audio.duration < DURATION: audio = audio.fx(vfx.loop, duration=DURATION)
        else: audio = audio.subclip(0, DURATION)
        final = final.set_audio(audio.audio_fadein(0.5).audio_fadeout(0.5))

    # --- 4. EXPORT ---
    final.write_videofile(OUTPUT_PATH, fps=24, codec='libx264', audio_codec='aac', preset='ultrafast', threads=4)
    print(f"✅ Vidéo TikTok prête : {OUTPUT_PATH}")

except Exception as e:
    print(f"❌ Erreur : {e}")

