# Photo filters

The camera composer includes optional face filters powered by the vendored
[Jeeliz FaceFilter](https://github.com/jeeliz/jeelizFaceFilter) browser engine.
The available filters are Sunglasses, Crown, and Puppy. Filter rendering is
client-side and the selected overlay is composited into the JPEG before it is
uploaded with the photo.

The same filter picker is available after choosing an image from the device. The
image is rendered into a local canvas stream for face tracking; the original
file is not sent anywhere until the user presses Send. If the browser does not
provide WebGL or canvas capture, the camera and upload flows remain available
without a filter.

Camera and location permissions are still required to send a photo. The camera
flow should be served over HTTPS outside local development, and users should
keep the face inside the preview for stable tracking.

The pinned Jeeliz browser build and `NN_DEFAULT.json` model live under
`frontend/public/vendor/jeeliz/`. Their Apache-2.0 license and source commit are
recorded beside the assets.
