export async function sameOriginLinkTarget(locator, beforeUrl, baseUrl) {
  const href = await locator
    .evaluate((element) => element.closest('a')?.href ?? '')
    .catch(() => '');
  if (!href) return null;
  let target;
  try {
    target = new URL(href, beforeUrl);
  } catch {
    return null;
  }
  if (target.origin !== baseUrl.origin || target.href === beforeUrl) return null;
  return target.href;
}

export async function clickAndWaitForNavigation(page, locator, baseUrl) {
  const beforeUrl = page.url();
  const linkTarget = await sameOriginLinkTarget(locator, beforeUrl, baseUrl);
  const navigation = linkTarget
    ? page.waitForURL((url) => url.toString() !== beforeUrl, { timeout: 10000 }).catch(() => {})
    : null;
  await locator.click({ timeout: 10000 });
  if (navigation) await navigation;
}
