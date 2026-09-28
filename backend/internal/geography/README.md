# Bundled country boundaries

`natural-earth-admin0-5.1.1.json.gz.b64` is a base64-wrapped gzip of a compact
geometry-only extract from Natural Earth 5.1.1
`ne_10m_admin_0_countries.geojson`. It retains each feature's `ADM0_A3` code and
polygon geometry. The source dataset is public domain. It is distributed under
the source's public-domain dedication:

- Dataset page:
  [Natural Earth Admin 0 Countries](https://www.naturalearthdata.com/downloads/10m-cultural-vectors/10m-admin-0-countries/)
- Upstream GeoJSON:
  [v5.1.1 country boundaries](https://github.com/nvkelso/natural-earth-vector/blob/v5.1.1/geojson/ne_10m_admin_0_countries.geojson)
- Upstream repository:
  [natural-earth-vector](https://github.com/nvkelso/natural-earth-vector)

The source contains 258 Admin 0 features. Its default boundary version uses de
facto control and treats some territories as separate areas. The compressed file
is embedded in the backend binary; it is not downloaded or refreshed at runtime.
To update it, regenerate the geometry-only extract from a reviewed Natural Earth
release, update this note and `docs/map-pins.md`, then validate the feature
count, unique codes, geometry types, and compressed file.
