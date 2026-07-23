# Photo filters

The camera composer includes optional face filters powered by the vendored
[Jeeliz FaceFilter](https://github.com/jeeliz/jeelizFaceFilter) browser engine.
The available lenses are Comedy glasses, Heart eyes, Puppy, Flower crown, and
Rainbow. They are local Canvas2D compositions driven by Jeeliz's face position,
scale, rotation, and mouth-opening expression data. Filter rendering is
client-side and the selected overlay is composited into the JPEG before it is
uploaded with the photo.

The same filter picker is available after choosing an image from the device. The
image is rendered into a local canvas stream for face tracking; the original
file is not sent anywhere until the user presses Send. If the browser does not
provide WebGL or canvas capture, the camera and upload flows remain available
without a filter.

Camera and location permissions are still required to send a photo. The camera
flow uses the front-facing camera for selfie-style lenses and should be served
over HTTPS outside local development. Users should keep the face well lit and
inside the preview for stable tracking; the picker smooths small movements and
uses the correct Jeeliz viewport coordinate orientation.

The pinned Jeeliz browser build and `NN_DEFAULT.json` model live under
`frontend/public/vendor/jeeliz/`. Their Apache-2.0 license and source commit are
recorded beside the assets.
