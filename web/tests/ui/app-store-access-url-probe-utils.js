const {NAMESPACE} = require("./app-store-access-url-config");
const {readOkJson} = require("./app-store-access-url-http");

async function readNamespacedList(page, apiPath, namespace = NAMESPACE, context) {
  const response = await page.context().request.get(`${apiPath}?namespace=${encodeURIComponent(namespace)}`);
  const body = await readOkJson(response, context);
  return Array.isArray(body.data) ? body.data : [];
}

// Keep the data-row-key and href selectors valid if future sample names contain CSS-sensitive characters.
function cssAttributeValue(value) {
  return String(value)
    .replace(/\\/g, "\\\\")
    .replace(/"/g, "\\\"")
    .replace(/\r/g, "\\D ")
    .replace(/\n/g, "\\A ")
    .replace(/\f/g, "\\C ");
}

module.exports = {
  cssAttributeValue,
  readNamespacedList,
};
