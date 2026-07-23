# Lens assets

All lens assets are served by the application. No camera frame or photo is sent
to an asset-generation service at runtime.

## Jeeliz puppy

`jeeliz-dog/` contains the ears and nose geometry, textures, and alpha map from
the Apache-2.0
[`jeelizFaceFilter` dog-face demo](https://github.com/jeeliz/jeelizFaceFilter/tree/master/demos/threejs/dog_face)
at commit `6f2695120b992511fd6cb1fd80c600bb957cd08c`. Its upstream license is
included at `jeeliz-dog/LICENSE`.

## Generated headpieces

The WebP files under `generated/` were created with OpenAI's built-in image
generation tool, keyed against a flat green background, converted to alpha, and
optimized as WebP. They are project assets; generation does not happen in the
application.

Prompt summaries:

- `disco-outlaw.webp`: a premium photorealistic mirrored disco cowboy hat,
  chrome stars, rhinestones, and a hot-pink feather; isolated front view.
- `red-flag-royalty.webp`: a ruby-and-gold crown made from theatrical satin red
  warning flags, chains, and gems; isolated front view.
- `bad-decisions.webp`: a neon adult-party halo combining flaming dice, a
  martini, metallic cards, and dark chrome; isolated front view.

Each source prompt required a uniform `#00ff00` chroma background, no person,
logos, watermark, or text, and a complete accessory with generous padding.
