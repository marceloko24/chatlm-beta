#!/usr/bin/env python3
from pathlib import Path
from PIL import Image, ImageDraw, ImageFont

def icon(size, path, maskable=False):
    img = Image.new("RGB", (size, size), "#0c0d11")
    d = ImageDraw.Draw(img)
    pad = int(size * (0.18 if maskable else 0.12))
    r = int(size * 0.22)
    d.rounded_rectangle([pad, pad, size - pad, size - pad], radius=r, fill="#161821")
    d.rounded_rectangle([pad, pad, size - pad, pad + int(size * 0.08)], radius=r // 3, fill="#e8c27a")
    font = None
    for fp in (
        "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
        "/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
    ):
        try:
            font = ImageFont.truetype(fp, int(size * 0.36))
            break
        except Exception:
            pass
    if font is None:
        font = ImageFont.load_default()
    text = "LM"
    bbox = d.textbbox((0, 0), text, font=font)
    tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
    x = (size - tw) / 2 - bbox[0]
    y = (size - th) / 2 - bbox[1] + int(size * 0.04)
    d.text((x, y), text, font=font, fill="#f4f1ea")
    path.parent.mkdir(parents=True, exist_ok=True)
    img.save(path, "PNG")

def main():
    out = Path("src/icons")
    icon(180, out / "apple-touch-icon.png")
    icon(192, out / "icon-192.png")
    icon(512, out / "icon-512.png")
    icon(512, out / "icon-512-maskable.png", maskable=True)
    print("icons written to", out)

if __name__ == "__main__":
    main()
