## gueoguess.me - Graphic Identity & Design Philosophy

Welcome to the visual world of gueoguess.me! Our graphic identity is designed to
be as engaging and enjoyable as the app itself, striking a balance between
playful accessibility and a clear focus on our core features: friendship, fun,
challenge, and geography.

### Core Principles

1. **Friendly & Approachable:** We want users to feel welcomed and excited to
   engage. Our designs avoid harsh lines, complex details, or overly serious
   tones. Everything should feel inviting and easy to understand at a glance.

2. **Playful & Fun:** Echoing the joy of discovery and friendly competition, our
   visuals incorporate cartoonish elements, vibrant colors, and dynamic
   compositions that suggest movement and interaction.

3. **Clear & Intuitive:** While playful, our designs are always functional.
   Icons are immediately recognizable, banners convey their purpose clearly, and
   backgrounds support content without distraction.

4. **Globally Inspired:** At the heart of gueoguess.me is geography. Subtle nods
   to maps, globes, and exploration are integrated throughout our assets,
   reinforcing the game's core mechanic.

5. **Consistent & Cohesive:** Every visual element, from the smallest icon to
   the largest banner, speaks the same language. This consistency builds trust,
   reinforces brand recognition, and ensures a seamless user experience.

### Key Visual Elements

- **Logo: The "High Five" Map Pins**
    - This iconic mark perfectly encapsulates our brand: two stylized map pins
      coming together in a friendly "high five" gesture over a simplified globe.
      It represents connection, friendship, and the global nature of our
      geographical challenges.

- **Color Palette:**
    - Our primary colors are vibrant and energetic. We use gradients extensively
      to add depth and a modern feel.
        - **Orange-Yellow Gradient:** Starts with a bright orange (e.g.,
          **`#FFB600`**) transitioning to a warm yellow (e.g., **`#FFD700`**).
          This is primarily used for the left map pin in the logo.
        - **Blue-Green Gradient:** Starts with a fresh blue (e.g.,
          **`#00C0FF`**) transitioning to a lively green (e.g., **`#00FF80`**).
          This is primarily used for the right map pin in the logo.
        - **Outline/Accent Dark Blue:** A clean, deep blue (e.g., **`#1A237E`**)
          is used for outlines, text, and other elements requiring strong
          contrast and legibility.
        - **Background White:** A pure white (e.g., **`#FFFFFF`**) or a very
          light grey (e.g., **`#F5F5F5`**) serves as our primary clean
          background.

- **Design Style: Simple Cartoon**
    - All assets feature clean lines, rounded edges, and simplified forms
      characteristic of a friendly cartoon aesthetic. Details are minimal but
      expressive, ensuring scalability and clarity across various screen sizes.
    - Gradients are used to add depth and vibrancy without resorting to complex
      shading, maintaining the "simple" aspect.

- **Imagery & Icons:**
    - Icons are designed to be immediately understandable, using universally
      recognized symbols (e.g., gear for settings, clock for time) but rendered
      in our unique cartoon style and color palette.
    - Elements like maps, globes, compasses, and subtle visual cues of travel or
      photography are subtly woven into many designs.
    - Illustrations, such as user avatars, are friendly and diverse, promoting
      inclusivity.

### Application

- **Icons:** Used for navigation, actions, and status indicators (e.g.,
  correct/incorrect guesses). They are crisp, clear, and primarily use our brand
  gradients within a simple outline.
- **Banners:** Employed for section headers, achievement displays, or
  promotional messages. They often feature dynamic shapes and integrate our logo
  or key thematic elements to convey excitement and purpose.
- **Backgrounds:** Subtle patterns (like faint map contours or scattered
  miniature icons) in muted versions of our palette are used to add visual
  interest without competing with foreground content.

### Product UI System

The application uses a refined-playful interpretation of this identity. Most of
the interface is composed from white and light-grey surfaces so that the brand
colors remain distinctive instead of becoming visual noise.

- **Typography:** Use the native system sans-serif stack. Headings are compact,
  bold, and slightly tightened; body copy remains neutral and highly legible.
- **Spacing:** Follow the shared 4px-based spacing tokens in `src/styles`. Avoid
  one-off spacing values unless a media or map surface requires them.
- **Shape:** Controls use 12px corners, content surfaces use 16px corners, and
  hero or modal surfaces use 24px corners. Fully rounded shapes are reserved for
  avatars, indicators, and compact badges.
- **Elevation:** Prefer a subtle border over a shadow. Small shadows identify
  interactive cards; larger shadows are reserved for dialogs and hero content.
- **Gradients:** Orange-yellow denotes the primary invitation or action.
  Blue-green denotes active, connected, selected, or progress states. Do not
  introduce unrelated feature gradients.
- **Illustration:** Use the branded PNG artwork for major moments and feature
  cues, not for routine controls. Utility actions use the consistent inline SVG
  icon set.
- **Responsive behavior:** Mobile layouts use edge-to-edge workspaces and a
  bottom group navigation bar. At 768px and above, content is constrained to a
  1120px frame and group navigation moves to a left rail.
- **Accessibility:** Interactive controls must be at least 44px, have a visible
  keyboard focus state, meet WCAG AA contrast, and respect reduced-motion user
  preferences.

### Protected Data Visualizations

The visual encoding of gameplay data is part of the product contract:

- Leaderboard score bars retain the blue-green gradient, with the first-place
  bar using the orange-yellow gradient.
- Gold (`#FFD700`), silver (`#C0C0C0`), and bronze (`#CD7F32`) ranking accents
  must not be reassigned.
- Leaflet tiles and default location markers remain unchanged. Guess markers
  remain orange (`#F59E0B`) with a white outline.
- Surrounding cards, spacing, and typography may evolve, but these colors and
  their meanings must remain stable.

By adhering to these principles and utilizing these visual elements, we ensure
that every interaction with gueoguess.me is a delightful, intuitive, and
memorable experience for our users.
