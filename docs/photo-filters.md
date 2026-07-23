# Photo filters

The camera composer includes an on-device augmented-reality lens system powered
by
[MediaPipe Face Landmarker](https://developers.google.com/edge/mediapipe/solutions/vision/face_landmarker)
and Three.js. Face Landmarker supplies 478 three-dimensional landmarks and 52
facial-expression values. The renderer anchors illuminated 3D geometry to the
eyes, cheeks, nose, mouth, forehead, and chin instead of estimating placement
from a face rectangle.

The catalog contains 15 effects plus the original image: Cyber visor, Crystal
crown, Neon kitty, 3D puppy, Inferno, Heavenly, Space cadet, Party pop,
Butterfly, Frog prince, Mecha, Masquerade, Ice queen, Pixel hero, and Superstar.
Several lenses react to expressions such as mouth opening, and animated
particles continue rendering while the camera is active.

The model, WebAssembly runtime, and rendering code are hosted by the
application. Camera frames and selected files remain in the browser; the
composited JPEG is the only image uploaded, and only after the user presses
Send. The same lens picker works for the live front-facing camera and JPEG, PNG,
or WebP files. Production's Content Security Policy permits WebAssembly
compilation with `wasm-unsafe-eval`; it does not permit general JavaScript
string evaluation.

WebGL is required for 3D rendering. MediaPipe attempts GPU inference first and
falls back to CPU inference when necessary. If tracking or rendering is
unavailable, the camera and original-file upload paths remain usable. Camera and
location permissions are still required to send a photo, and camera access
requires HTTPS outside local development.

The pinned Face Landmarker model and provenance live under
`frontend/public/vendor/mediapipe/`. MediaPipe and its model use Apache-2.0;
Three.js uses the MIT license.
