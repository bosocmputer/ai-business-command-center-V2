"""Draws the owner's LINE menu picture (2500x843, four buttons) into richmenu.png next to this file.

  python3 make_richmenu.py [output.png]

The four buttons and the texts they send are defined in backend/internal/line/richmenu.go (DefaultMenuButtons): keep the labels here in
step with it. Needs Pillow and a Thai bold font (Thonburi on macOS; change FONT below for another machine).
"""
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

FONT, FONT_INDEX = "/System/Library/Fonts/Supplemental/Thonburi.ttc", 1  # Thonburi Bold
WIDTH, HEIGHT, SCALE = 2500, 843, 2
NAVY, TEAL, AMBER, SLATE, GROUND, WHITE = "#1F2A44", "#1FA39A", "#F5A524", "#3A4663", "#EEF3F2", "#FFFFFF"

# (first line, second line, tile colour, ink colour)
BUTTONS = [
    ("ยอดขาย", "เมื่อวาน", NAVY, WHITE),
    ("ยอดขายวันนี้", "สดล่าสุด", TEAL, WHITE),
    ("สินค้า", "ใกล้หมด", AMBER, NAVY),
    ("เปลี่ยน", "ร้าน", SLATE, WHITE),
]


def s(value):
    return int(value * SCALE)


def icon_sales_yesterday(draw, cx, cy, ink):
    # three bars growing to the right on a baseline
    base = cy + 95
    for index, (height, left) in enumerate([(80, -110), (135, -40), (195, 30)]):
        draw.rounded_rectangle([s(cx + left), s(base - height), s(cx + left + 56), s(base)], radius=s(8), fill=ink)
    draw.rounded_rectangle([s(cx - 135), s(base + 12), s(cx + 115), s(base + 24)], radius=s(6), fill=ink)


def icon_sales_today(draw, cx, cy, ink):
    # a clock face: "right now"
    r = 105
    draw.ellipse([s(cx - r), s(cy - r + 10), s(cx + r), s(cy + r + 10)], outline=ink, width=s(16))
    draw.line([s(cx), s(cy + 10), s(cx), s(cy - 52)], fill=ink, width=s(16))
    draw.line([s(cx), s(cy + 10), s(cx + 50), s(cy + 40)], fill=ink, width=s(16))
    draw.ellipse([s(cx - 13), s(cy - 3), s(cx + 13), s(cy + 23)], fill=ink)


def icon_low_stock(draw, cx, cy, ink):
    # a box with a warning triangle in front of it
    draw.rounded_rectangle([s(cx - 125), s(cy - 40), s(cx + 55), s(cy + 105)], radius=s(12), outline=ink, width=s(15))
    draw.line([s(cx - 125), s(cy + 5), s(cx + 55), s(cy + 5)], fill=ink, width=s(15))
    triangle = [(cx + 60, cy - 100), (cx + 135, cy + 30), (cx - 15, cy + 30)]
    draw.polygon([(s(x), s(y)) for x, y in triangle], fill=ink)
    draw.polygon([(s(x), s(y)) for x, y in [(cx + 60, cy - 70), (cx + 108, cy + 14), (cx + 12, cy + 14)]], fill=AMBER)
    draw.rounded_rectangle([s(cx + 55), s(cy - 45), s(cx + 65), s(cy - 10)], radius=s(4), fill=ink)
    draw.ellipse([s(cx + 53), s(cy - 3), s(cx + 67), s(cy + 11)], fill=ink)


def icon_change_shop(draw, cx, cy, ink):
    # a shop front: awning and body
    draw.rounded_rectangle([s(cx - 105), s(cy - 15), s(cx + 105), s(cy + 100)], radius=s(10), outline=ink, width=s(15))
    for index in range(4):
        left = cx - 115 + index * 58
        draw.pieslice([s(left), s(cy - 105), s(left + 58), s(cy - 15 + 30)], 0, 180, fill=ink)
        draw.rectangle([s(left), s(cy - 105), s(left + 58), s(cy - 61)], fill=ink)
    draw.rounded_rectangle([s(cx - 22), s(cy + 28), s(cx + 22), s(cy + 100)], radius=s(6), fill=ink)


ICONS = [icon_sales_yesterday, icon_sales_today, icon_low_stock, icon_change_shop]


def main():
    out = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(__file__).with_name("richmenu.png")
    image = Image.new("RGB", (s(WIDTH), s(HEIGHT)), GROUND)
    draw = ImageDraw.Draw(image)
    big = ImageFont.truetype(FONT, s(98), index=FONT_INDEX)
    gap, tile_width = 14, WIDTH // len(BUTTONS)
    for index, (first, second, colour, ink) in enumerate(BUTTONS):
        left, right = index * tile_width + gap, (index + 1) * tile_width - gap
        draw.rounded_rectangle([s(left), s(gap), s(right), s(HEIGHT - gap)], radius=s(36), fill=colour)
        cx = (left + right) // 2
        ICONS[index](draw, cx, 255, ink)
        for row, text in enumerate((first, second)):
            draw.text((s(cx), s(560 + row * 125)), text, font=big, fill=ink, anchor="mm")
    image = image.resize((WIDTH, HEIGHT), Image.LANCZOS)
    image.save(out, optimize=True)
    print(out, out.stat().st_size, "bytes")


if __name__ == "__main__":
    main()
