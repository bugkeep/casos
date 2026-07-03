async function markDeploymentAccessUrlClick(page, target) {
  await page.evaluate(({rowKey, accessUrl, accessUrlType}) => {
    const previousStyle = document.getElementById("casos-access-url-click-style");
    if (previousStyle) {
      previousStyle.remove();
    }
    const previousBanner = document.getElementById("casos-access-url-click-banner");
    if (previousBanner) {
      previousBanner.remove();
    }
    document
      .querySelectorAll(".casos-access-url-click-row,.casos-access-url-click-link")
      .forEach(element => element.classList.remove("casos-access-url-click-row", "casos-access-url-click-link"));

    const style = document.createElement("style");
    style.id = "casos-access-url-click-style";
    style.textContent = `
      .casos-access-url-click-row {
        box-shadow: inset 0 0 0 3px #faad14 !important;
        background: rgba(250, 173, 20, 0.10) !important;
      }
      .casos-access-url-click-link {
        outline: 3px solid #faad14 !important;
        outline-offset: 3px !important;
        border-radius: 4px !important;
        background: #fffbe6 !important;
        color: #ad6800 !important;
      }
      #casos-access-url-click-banner {
        position: fixed;
        right: 24px;
        bottom: 24px;
        z-index: 2147483646;
        max-width: min(520px, calc(100vw - 48px));
        box-sizing: border-box;
        padding: 10px 12px;
        border: 2px solid #faad14;
        border-radius: 8px;
        background: #fffbe6;
        color: #613400;
        box-shadow: 0 10px 24px rgba(97, 52, 0, 0.20);
        font-family: Arial, sans-serif;
        font-size: 13px;
        line-height: 1.4;
        overflow-wrap: anywhere;
      }
      #casos-access-url-click-banner strong {
        display: block;
        margin-bottom: 2px;
      }
    `;
    document.head.appendChild(style);

    const row = Array.from(document.querySelectorAll("tr[data-row-key]"))
      .find(element => element.getAttribute("data-row-key") === rowKey);
    const link = row
      ? Array.from(row.querySelectorAll("a[href]")).find(element => element.getAttribute("href") === accessUrl)
      : null;
    if (row) {
      row.classList.add("casos-access-url-click-row");
    }
    if (link) {
      link.classList.add("casos-access-url-click-link");
    }

    const banner = document.createElement("div");
    banner.id = "casos-access-url-click-banner";
    const title = document.createElement("strong");
    title.textContent = `Clicking ${accessUrlType || "Access"} URL from Deployments`;
    const url = document.createElement("div");
    url.textContent = accessUrl || "url-not-recorded";
    banner.appendChild(title);
    banner.appendChild(url);
    document.body.appendChild(banner);
  }, target);
}

module.exports = {
  markDeploymentAccessUrlClick,
};
