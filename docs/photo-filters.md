# Photo filters

The camera composer includes an on-device augmented-reality lens system powered
by
[MediaPipe Face Landmarker](https://developers.google.com/edge/mediapipe/solutions/vision/face_landmarker)
and Three.js. Face Landmarker supplies 478 three-dimensional landmarks and 52
facial-expression values. The renderer anchors illuminated 3D geometry to the
eyes, cheeks, nose, mouth, forehead, and chin instead of estimating placement
from a face rectangle.

The catalog contains 24 effects plus the original image. It combines animated
Three.js accessories, the original Jeeliz dog ears and nose, generated
headpieces, full-face frames, and three live face deformations. The adult-party
set includes HR nightmare, Toxic ex, Tax fraud, Disco outlaw, Red flag royalty,
and Bad decisions. The deliberately exaggerated deformation modes are Ego
inflation, Doomscroll damage, and Budget facelift. Several lenses react to
expressions such as mouth opening, and animated particles continue rendering
while the camera is active.

The compact, camera-first lens rail uses circular previews and keeps the
selected lens name in a small chip above the rail. It supports touch swiping,
mouse dragging, mouse-wheel scrolling, and desktop previous/next controls. The
compact preview artwork is requested only after a lens is focused, hovered, or
tapped, so opening the rail does not download the whole catalog and each
carousel tile transfers only a thumbnail-sized WebP. The text tool adds an
editable banner before or after capture, with Classic, Neon, and Clean themes
and adjustable vertical placement. Banner text is rendered into the final JPEG
rather than uploaded as separate metadata.

The model, WebAssembly runtime, and rendering code are hosted by the
application. Camera frames and selected files remain in the browser; the
composited JPEG is the only image uploaded, and only after the user presses
Send. The same lens picker works for the live front-facing camera and JPEG, PNG,
or WebP files. The live camera preview and its lens overlay use the camera's
natural orientation for the environment-facing camera. The user-facing live
preview and its lens overlay are mirrored for natural selfie framing, while the
captured canvas and uploaded media are never mirrored. Production's Content
Security Policy permits WebAssembly compilation with `wasm-unsafe-eval`; it does
not permit general JavaScript string evaluation.

WebGL is required for 3D rendering. MediaPipe attempts GPU inference first and
falls back to CPU inference when necessary. If tracking or rendering is
unavailable, the camera and original-file upload paths remain usable. Camera and
location permissions are still required to send a photo, and camera access
requires HTTPS outside local development. After the application becomes idle on
a visible page, it imports the face-tracking module and warms one landmarker in
the background. The first lens activation reuses that warmed instance, while a
failed or interrupted preload is retried by normal camera startup; this keeps
the startup path responsive without making lens support a prerequisite for using
the rest of the application.

The pinned Face Landmarker model and provenance live under
`frontend/public/vendor/mediapipe/`. Lens asset provenance lives under
`frontend/public/lenses/`. MediaPipe, its model, and the reused Jeeliz demo
assets use Apache-2.0; Three.js uses the MIT license.
