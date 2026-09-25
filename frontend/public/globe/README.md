# Earth texture

`earth-8192.jpg` is the 8192 × 4096 equirectangular Earth texture published by
NASA's [Scientific Visualization Studio](https://svs.gsfc.nasa.gov/3615/),
downloaded unchanged from
[`flat_earth_Largest_still.0330.jpg`](https://svs.gsfc.nasa.gov/vis/a000000/a003600/a003615/flat_earth_Largest_still.0330.jpg).
It uses the Blue Marble: Next Generation data courtesy of Reto Stöckli
(NASA/GSFC) and NASA's Earth Observatory. The land imagery is derived from MODIS
observations collected in 2004. The renderer selects this asset only when WebGL
reports an 8192-pixel texture limit; `earth.jpg` remains the bundled 2048 × 1024
fallback for devices with smaller limits. The fallback is the texture
distributed with the
[Three.js planet examples](https://github.com/mrdoob/three.js/tree/dev/examples/textures/planets),
downloaded unchanged from
[`earth_atmos_2048.jpg`](https://threejs.org/examples/textures/planets/earth_atmos_2048.jpg).

Credit: NASA/Goddard Space Flight Center Scientific Visualization Studio; Blue
Marble Next Generation data courtesy of Reto Stöckli (NASA/GSFC) and NASA Earth
Observatory. The bundled texture is drawn immediately and remains visible
offline. When the view needs more detail, the globe requests visible NASA GIBS
500m Blue Marble tiles, then standard OpenStreetMap raster tiles for closer
views (up to zoom 19). It requests only tiles intersecting the current view,
keeps requests and decoded textures within a small viewport budget, and uses
normal browser HTTP caching. A request reveals the approximate visible map area
and the browser's normal network metadata to the tile provider. If either tile
source is unavailable, the bundled Earth texture remains usable.

NASA GIBS imagery credit: “We acknowledge the use of imagery provided by
services from NASA's Global Imagery Browse Services (GIBS), part of NASA's Earth
Science Data and Information System (ESDIS).” See the
[NASA GIBS documentation](https://earthdata.nasa.gov/gibs).

Detailed map data credit:
[© OpenStreetMap contributors](https://www.openstreetmap.org/copyright). The
standard tile service is best-effort and subject to its
[tile usage policy](https://operations.osmfoundation.org/policies/tiles/).
