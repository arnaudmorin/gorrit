import re, sys
from PIL import Image, ImageDraw, ImageFont

FONT = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
BOLDFONT = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"
BASE16 = ["#000000","#cc0000","#4e9a06","#c4a000","#3465a4","#75507b","#06989a","#d3d7cf",
          "#555753","#ef2929","#8ae234","#fce94f","#729fcf","#ad7fa8","#34e2e2","#eeeeec"]
def c256(n):
    if n < 16: return BASE16[n]
    if n < 232:
        n -= 16; r, g, b = n//36, (n//6)%6, n%6
        f = lambda v: 0 if v == 0 else 55 + v*40
        return "#%02x%02x%02x" % (f(r), f(g), f(b))
    v = 8 + (n-232)*10; return "#%02x%02x%02x" % (v, v, v)

FG, BG = "#d0d0d0", "#1c1c1c"
sgr = re.compile(r"\x1b\[([0-9;]*)m|\x1b\[[0-9;?]*[A-Za-z]")
cells = []  # list of rows; row = list of (ch, fg, bg, bold, underline)
def parse(text):
    st = dict(fg=FG, bg=BG, bold=False, ul=False, rev=False)
    row = []
    rows = [row]
    i = 0
    for m in sgr.finditer(text):
        for ch in text[i:m.start()]:
            if ch == "\n": row = []; rows.append(row)
            elif ch == "\r": pass
            else:
                fg, bg = (st["bg"], st["fg"]) if st["rev"] else (st["fg"], st["bg"])
                row.append((ch, fg, bg, st["bold"], st["ul"]))
        i = m.end()
        if m.group(1) is None: continue
        p = [int(x) if x else 0 for x in m.group(1).split(";")] or [0]
        k = 0
        while k < len(p):
            n = p[k]
            if n == 0: st.update(fg=FG, bg=BG, bold=False, ul=False, rev=False)
            elif n == 1: st["bold"] = True
            elif n == 4: st["ul"] = True
            elif n == 7: st["rev"] = True
            elif n == 22: st["bold"] = False
            elif n == 24: st["ul"] = False
            elif n == 27: st["rev"] = False
            elif 30 <= n <= 37: st["fg"] = BASE16[n-30]
            elif 90 <= n <= 97: st["fg"] = BASE16[n-90+8]
            elif 40 <= n <= 47: st["bg"] = BASE16[n-40]
            elif 100 <= n <= 107: st["bg"] = BASE16[n-100+8]
            elif n == 39: st["fg"] = FG
            elif n == 49: st["bg"] = BG
            elif n in (38, 48):
                key = "fg" if n == 38 else "bg"
                if p[k+1] == 5: st[key] = c256(p[k+2]); k += 2
                elif p[k+1] == 2: st[key] = "#%02x%02x%02x" % tuple(p[k+2:k+5]); k += 4
            k += 1
    for ch in text[i:]:
        if ch == "\n": row = []; rows.append(row)
        else: row.append((ch, st["fg"], st["bg"], st["bold"], st["ul"]))
    return rows

def render(rows, out, cols=None):
    font = ImageFont.truetype(FONT, 15); bold = ImageFont.truetype(BOLDFONT, 15)
    cw, chh = font.getbbox("M")[2], 19
    cols = cols or max(len(r) for r in rows)
    img = Image.new("RGB", (cw*cols + 20, chh*len(rows) + 20), BG)
    d = ImageDraw.Draw(img)
    for y, r in enumerate(rows):
        x = 0
        for ch, fg, bg, b, ul in r:
            w = 2 if ord(ch) > 0x2E7F and ord(ch) not in range(0x2500, 0x2600) else 1
            px, py = 10 + x*cw, 10 + y*chh
            if bg != BG: d.rectangle([px, py, px+cw*w, py+chh], fill=bg)
            d.text((px, py), ch, font=bold if b else font, fill=fg)
            if ul: d.line([px, py+chh-2, px+cw*w, py+chh-2], fill=fg)
            x += w
    img.save(out)

if __name__ == "__main__":
    text = open(sys.argv[1], encoding="utf-8").read()
    render(parse(text), sys.argv[2])
