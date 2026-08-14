export async function probe(page) {
  return page.evaluate(async () => {
    const result = { secure_context: window.isSecureContext, camera: { granted: false, usable: false }, geolocation: { granted: false, usable: false } };
    try { result.camera.granted = (await navigator.permissions.query({ name: "camera" })).state === "granted"; } catch {}
    try { result.geolocation.granted = (await navigator.permissions.query({ name: "geolocation" })).state === "granted"; } catch {}
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false });
      result.camera.usable = stream.getVideoTracks().length > 0;
      stream.getTracks().forEach((track) => track.stop());
    } catch {}
    await new Promise((resolve) => {
      if (!navigator.geolocation) return resolve();
      navigator.geolocation.getCurrentPosition(
        (position) => { result.geolocation.usable = Number.isFinite(position.coords.latitude) && Number.isFinite(position.coords.longitude); resolve(); },
        () => resolve(),
        { timeout: 3000, maximumAge: 0 },
      );
    });
    return result;
  });
}

export function fakeLocation() {
  const latitude = Number(process.env.QA_FAKE_LATITUDE || 48.8566);
  const longitude = Number(process.env.QA_FAKE_LONGITUDE || 2.3522);
  const accuracy = Number(process.env.QA_FAKE_LOCATION_ACCURACY || 10);
  if (!Number.isFinite(latitude) || latitude < -90 || latitude > 90) throw new Error("QA_FAKE_LATITUDE must be between -90 and 90");
  if (!Number.isFinite(longitude) || longitude < -180 || longitude > 180) throw new Error("QA_FAKE_LONGITUDE must be between -180 and 180");
  if (!Number.isFinite(accuracy) || accuracy <= 0) throw new Error("QA_FAKE_LOCATION_ACCURACY must be positive");
  return { latitude, longitude, accuracy };
}
