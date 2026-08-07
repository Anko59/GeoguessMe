# Custom UI icons

The application replaced its last emoji controls with branded artwork so every
routine control speaks the same visual language as the logo, banners, reactions,
and rank badges. This page documents the icon sets, their display rules, and how
the artwork is produced and maintained.

## Icon sets

### Camera controls — `frontend/public/ui/`

The camera composer's utility buttons each use a dedicated brand icon on a dark
glass pill or in the options menu:

| Asset               | Replaces | Control                                |
| ------------------- | -------- | -------------------------------------- |
| `switch-camera.png` | 🔄       | Switch between front and back camera   |
| `lenses-toggle.png` | 🎭       | Show / hide the lens rail              |
| `add-text.png`      | Aa       | Open the text banner editor            |
| `options-gear.png`  | ⚙️       | Open the challenge options menu        |
| `hide-location.png` | 🕵️       | "Hide my location" option in the menu  |
| `crown.png`         | 👑       | Profile "top of the ladder" decoration |
| `medal-gold.png`    | 🥇       | Leaderboard first place                |
| `medal-silver.png`  | 🥈       | Leaderboard second place               |
| `medal-bronze.png`  | 🥉       | Leaderboard third place                |

The leaderboard medals keep the protected ranking colors: gold (`#FFD700`),
silver (`#C0C0C0`), and bronze (`#CD7F32`) accents are preserved, and the
first-place bar keeps the orange-yellow gradient, so the data visualization
contract in `frontend/public/Identity.md` is unchanged.

### Lens catalog — `frontend/public/lenses/icons/`

Every entry in the lens rail (25 including "Original") has a matching brand icon
named `{lens-id}.png` (for example `hr-nightmare.png`, `toxic-ex.png`). The icon
sits in the accent-colored circular tile; when a lens has generated preview
artwork, the tile swaps to the thumbnail WebP on focus, hover, or tap, exactly
as before. The AR lens effects themselves (`frontend/public/lenses/generated/`)
are untouched — only the picker icons were replaced.

## Artwork generation

All icons are generated with the `minimax/image-01` model on Replicate using a
single style recipe so the set looks consistent, the same recipe used for the
[rank badges](rank-badges.md):

- flat cartoon vector icon, centered, symmetric, clean edges
- vibrant orange-to-yellow and blue-to-green brand gradients with deep navy
  (`#1A237E`) outlines
- isolated on a pure white background
- no text, no letters, no numbers, no watermark
- no border, no frame, no drop shadow

The Replicate token is read from `REPLICATE_API_TOKEN` in the developer's shell
environment. Generation is a one-time manual step, not part of the build: the
finished transparent PNGs are committed as 256×256 assets.

Because `image-01` outputs JPEG (no alpha channel), generated images are
post-processed once before being committed: a two-pass border flood fill removes
the white background and the JPEG compression ring around the artwork,
near-white pixels adjacent to transparency are snapped to alpha 0, enclosed
near-white regions above a size threshold are removed, and the result is trimmed
to its solid content, padded with a uniform margin, centered, and resized to
256×256. If a generation comes back with a gray vignette instead of a white
background it must be regenerated with the same post-processing applied.

## Display rules

- Camera control icons render at 1.2–1.45rem inside their existing buttons;
  `alt=""` with the button's `aria-label` keeps the controls accessible.
- Lens icons render at 2rem inside the 3.55rem accent tile.
- Medals render at 1.6rem, with the gold medal slightly larger (1.9rem) to
  preserve the first-place emphasis.
- The crown renders inline at 1.15rem in the profile progress text.

## Maintenance

- Replacements follow the same recipe and post-processing; verify transparency
  and margins programmatically and review the result before committing.
- Regeneration is a one-off manual step, never part of the build or CI.
