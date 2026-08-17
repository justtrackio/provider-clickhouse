#!/usr/bin/env python3
"""Generate the GitHub social preview card (1280x640) for provider-clickhouse.

GitHub renders the social preview at roughly 2:1, so 1280x640 fills it without
cropping. Kept deliberately text-only and high contrast: these cards are usually
seen small, in a Slack or LinkedIn unfurl, so the headline has to survive being
shrunk to a few hundred pixels wide.
"""

from PIL import Image, ImageDraw, ImageFont

W, H = 1280, 640

BG = (11, 14, 20)
BG_PANEL = (17, 22, 31)
YELLOW = (255, 204, 1)      # ClickHouse accent
BLUE = (56, 139, 253)       # Crossplane-ish accent
WHITE = (240, 246, 252)
GREY = (139, 148, 158)

SANS_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
SANS = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
MONO = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
MONO_BOLD = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf"


def f(path, size):
    return ImageFont.truetype(path, size)


def main():
    img = Image.new("RGB", (W, H), BG)
    d = ImageDraw.Draw(img)

    # Left accent bar in ClickHouse yellow.
    d.rectangle([0, 0, 14, H], fill=YELLOW)

    # Subtle panel behind the lower half to separate headline from details.
    d.rectangle([14, 430, W, H], fill=BG_PANEL)

    x = 72

    # Eyebrow.
    d.text((x, 74), "CROSSPLANE PROVIDER", font=f(SANS_BOLD, 24), fill=YELLOW)

    # Title.
    d.text((x, 118), "provider-clickhouse", font=f(MONO_BOLD, 68), fill=WHITE)

    # Headline: the actual value proposition, two lines, large.
    d.text((x, 218), "ClickHouse Cloud and ClickStack", font=f(SANS_BOLD, 44), fill=WHITE)
    d.text((x, 274), "as Kubernetes resources", font=f(SANS_BOLD, 44), fill=WHITE)

    # Supporting line.
    d.text(
        (x, 352),
        "Saved searches, dashboards and alerts in Git \u2014 reconciled, reviewed, reversible.",
        font=f(SANS, 25),
        fill=GREY,
    )

    # Stat row inside the panel.
    stats = [
        ("25", "resources"),
        ("9", "ClickStack kinds"),
        ("50", "CRDs"),
        ("v2", "Crossplane ready"),
    ]
    sx = x
    for num, label in stats:
        nf = f(SANS_BOLD, 46)
        lf = f(SANS, 19)
        d.text((sx, 468), num, font=nf, fill=YELLOW)
        d.text((sx, 524), label, font=lf, fill=GREY)
        width = max(d.textlength(num, font=nf), d.textlength(label, font=lf))
        sx += int(width) + 66

    # Install line, bottom.
    d.text(
        (x, 578),
        "ghcr.io/justtrackio/provider-clickhouse",
        font=f(MONO, 23),
        fill=BLUE,
    )

    img.save(".github/social-preview.png", "PNG", optimize=True)
    print(f"wrote .github/social-preview.png {img.size[0]}x{img.size[1]}")


if __name__ == "__main__":
    main()
