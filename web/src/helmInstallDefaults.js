import yaml from "js-yaml";

// Filter prototype-pollution keys before merging YAML-derived objects.
const dangerousMergeKeys = new Set(["__proto__", "constructor", "prototype"]);

function isPlainObject(value) {
  return Object.prototype.toString.call(value) === "[object Object]";
}

function sanitizePlainObject(value, depth = 0, maxDepth = 100) {
  if (depth > maxDepth) {
    return {};
  }

  if (Array.isArray(value)) {
    return value.map((item) => sanitizePlainObject(item, depth + 1, maxDepth));
  }

  if (!isPlainObject(value)) {
    return value;
  }

  const sanitized = {};
  Object.entries(value).forEach(([key, child]) => {
    if (dangerousMergeKeys.has(key)) {
      return;
    }
    sanitized[key] = sanitizePlainObject(child, depth + 1, maxDepth);
  });
  return sanitized;
}

function deepMerge(target, source, depth = 0, maxDepth = 100) {
  if (!isPlainObject(target) || !isPlainObject(source) || depth > maxDepth) {
    return source;
  }

  const merged = {...target};
  Object.entries(source).forEach(([key, value]) => {
    // Keep filtering here as defense-in-depth even though parsed YAML is sanitized first.
    if (dangerousMergeKeys.has(key)) {
      return;
    }
    if (isPlainObject(value) && isPlainObject(merged[key])) {
      merged[key] = deepMerge(merged[key], value, depth + 1, maxDepth);
    } else {
      merged[key] = value;
    }
  });
  return merged;
}

export function buildRecommendedValues(baseValuesYAML, releaseName) {
  const trimmed = (baseValuesYAML || "").trim();
  let parsed = {};
  let defaultsApplied = false;

  if (trimmed !== "") {
    try {
      parsed = yaml.load(trimmed, {schema: yaml.JSON_SCHEMA, noRefs: true}) ?? {};
    } catch (_) {
      return {yaml: baseValuesYAML, defaultsApplied: false};
    }
    if (!isPlainObject(parsed)) {
      return {yaml: baseValuesYAML, defaultsApplied: false};
    }
    parsed = sanitizePlainObject(parsed);
  }

  const overrides = {};
  if (releaseName) {
    overrides.fullnameOverride = releaseName;
    defaultsApplied = true;
  }

  if (isPlainObject(parsed.service)) {
    overrides.service = {...(overrides.service || {}), type: "NodePort"};
    defaultsApplied = true;
  }

  if (!defaultsApplied) {
    return {yaml: baseValuesYAML, defaultsApplied: false};
  }

  return {
    yaml: yaml.dump(deepMerge(parsed, overrides), {lineWidth: -1}),
    defaultsApplied,
  };
}
