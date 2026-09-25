# Custom UI icons

The application replaced its last emoji controls with branded artwork so every
routine control speaks the same visual language as the logo, banners, reactions,
and rank badges. This page documents the icon sets, their display rules, and how
the artwork is produced and maintained.

## Icon sets

### Camera and utility controls — `frontend/public/ui/`

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

### Feature artwork — `frontend/public/`

The group header uses two additional branded illustrations:

| Asset                    | Control                          |
| ------------------------ | -------------------------------- |
| `globe_feature_icon.png` | Open the group's challenge globe |
| `party_mode_icon.png`    | Start or show Party Time         |

These feature icons use the same flat, rounded construction as the first two
generations of artwork. They are kept separate from the compact utility icons
and the chat reaction vocabulary, even when they share the same brand palette.

### Chat reactions — `frontend/public/reactions/`

The chat reaction picker contains 24 branded reaction assets, including thumbs
up/down, love, laughing, crying, kissing, surprise, anger, confusion, clapping,
prayer, fire, and party reactions. The picker is one horizontally scrollable
row, and the authenticated group member's aggregate reaction usage orders the
most-used reactions first. Ties and unused reactions use the curated fallback
order in `frontend/src/components/chat/reactionOptions.ts`. Legacy emoji keys
remain readable and valid during the migration window.

### Lens catalog — `frontend/public/lenses/icons/`

Every entry in the lens rail (25 including "Original") has a matching brand icon
named `{lens-id}.png` (for example `hr-nightmare.png`, `toxic-ex.png`). The icon
sits in the accent-colored circular tile; when a lens has generated preview
artwork, the tile swaps to the thumbnail WebP on focus, hover, or tap, exactly
as before. The AR lens effects themselves (`frontend/public/lenses/generated/`)
are untouched — only the picker icons were replaced.

## Artwork generation

The committed artwork follows a shared style recipe inspired by the original
brand illustrations and the flatter second-generation assets:

- flat cartoon vector icon, centered, symmetric, clean edges
- vibrant orange-to-yellow and blue-to-green brand gradients with deep navy
  (`#1A237E`) outlines
- isolated with a transparent background
- no text, no letters, no numbers, no watermark
- no border, no frame, no drop shadow

Generation is a one-time manual step, not part of the build. Before committing,
each replacement is trimmed to its solid content, padded with a uniform
transparent margin, centered, resized to 256×256, and checked for preserved
alpha.

If a generation contains a background halo or gray vignette, it must be
regenerated or cleaned before review.

## Display rules

- Camera control icons render at 1.2–1.45rem inside their existing buttons;
  `alt=""` with the button's `aria-label` keeps the controls accessible.
- Lens icons render at 2rem inside the 3.55rem accent tile.
- Medals render at 1.6rem, with the gold medal slightly larger (1.9rem) to
  preserve the first-place emphasis.
- The crown renders inline at 1.15rem in the profile progress text.
- Feature artwork renders at the size required by its owning control; the
  compact header controls use the same 44px hit area and accessible labels as
  before.

## Maintenance

- Replacements follow the same recipe and post-processing; verify transparency
  and margins programmatically and review the result before committing.
- Regeneration is a one-off manual step, never part of the build or CI.
