# Earth texture

`earth-8192.jpg` is the 8192 × 4096 equirectangular Earth texture published by
NASA's [Scientific Visualization Studio](https://svs.gsfc.nasa.gov/3615/),
downloaded unchanged from
[`flat_earth_Largest_still.0330.jpg`](https://svs.gsfc.nasa.gov/vis/a000000/a003600/a003615/flat_earth_Largest_still.0330.jpg).
It uses the Blue Marble: Next Generation data courtesy of Reto Stöckli
(NASA/GSFC) and NASA's Earth Observatory. The renderer selects this asset only
when WebGL reports an 8192-pixel texture limit; `earth.jpg` remains the bundled
2048 × 1024 fallback for devices with smaller limits. The fallback is the
texture distributed with the
[Three.js planet examples](https://github.com/mrdoob/three.js/tree/dev/examples/textures/planets),
downloaded unchanged from
[`earth_atmos_2048.jpg`](https://threejs.org/examples/textures/planets/earth_atmos_2048.jpg).

Credit: NASA/Goddard Space Flight Center Scientific Visualization Studio; Blue
Marble Next Generation data courtesy of Reto Stöckli (NASA/GSFC) and NASA Earth
Observatory. The assets are served locally and do not contact a third-party tile
or imagery service at runtime.
