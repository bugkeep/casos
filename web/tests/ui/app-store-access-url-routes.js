function escapeRegExp(value) {
  return String(value).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function routeUrlPattern(route) {
  return new RegExp(`${escapeRegExp(route)}\\/?$`);
}

module.exports = {
  escapeRegExp,
  routeUrlPattern,
};
