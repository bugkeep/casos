// Kubernetes quantity strings ("100m", "512Mi", "1.5", "1e3"). The form needs
// to parse them, not just pattern-match, so it can show what a value actually
// means before the user submits it.

const BINARY = {Ki: 1024, Mi: 1024 ** 2, Gi: 1024 ** 3, Ti: 1024 ** 4, Pi: 1024 ** 5, Ei: 1024 ** 6};
const DECIMAL = {n: 1e-9, u: 1e-6, m: 1e-3, "": 1, k: 1e3, M: 1e6, G: 1e9, T: 1e12, P: 1e15, E: 1e18};

// The units people reach for out of habit, and what they meant instead.
const MISTAKES = {
  KB: "Ki", MB: "Mi", GB: "Gi", TB: "Ti",
  kb: "Ki", mb: "Mi", gb: "Gi", tb: "Ti",
  Kb: "Ki", Mb: "Mi", Gb: "Gi", Tb: "Ti",
  b: "", B: "", K: "Ki", g: "Gi", i: "", Mib: "Mi", Gib: "Gi", MiB: "Mi", GiB: "Gi", KiB: "Ki",
};

const SHAPE = /^([+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+))([eE][+-]?[0-9]+|[a-zA-Z]*)$/;

/**
 * Returns {value, error}. `value` is in cores for CPU and in bytes for memory;
 * both are null when the input is empty or unparseable.
 */
export function parseQuantity(text) {
  const raw = String(text ?? "").trim();
  if (raw === "") {
    return {value: null, error: null};
  }

  const match = SHAPE.exec(raw);
  if (!match) {
    return {value: null, error: "Not a valid quantity — try 100m, 0.5, 256Mi or 1Gi"};
  }

  const [, mantissa, suffix] = match;
  const number = Number(mantissa);
  if (!Number.isFinite(number)) {
    return {value: null, error: "Not a valid number"};
  }

  let multiplier;
  if (/^[eE]/.test(suffix)) {
    multiplier = 10 ** Number(suffix.slice(1));
  } else if (suffix in BINARY) {
    multiplier = BINARY[suffix];
  } else if (suffix in DECIMAL) {
    multiplier = DECIMAL[suffix];
  } else if (suffix in MISTAKES) {
    const meant = MISTAKES[suffix];
    return {value: null, error: `"${suffix}" is not a Kubernetes unit — did you mean ${mantissa}${meant}?`};
  } else {
    return {value: null, error: `Unknown unit "${suffix}" — use m, k, M, G or Ki, Mi, Gi`};
  }

  const value = number * multiplier;
  if (value < 0) {
    return {value, error: "Must not be negative"};
  }
  return {value, error: null};
}

function trim(number, digits = 2) {
  return String(Number(number.toFixed(digits)));
}

export function formatCores(cores) {
  if (cores >= 1) {
    return `${trim(cores, 3)} ${cores === 1 ? "core" : "cores"}`;
  }
  return `${trim(cores, 3)} cores (${trim(cores * 1000, 0)}m)`;
}

export function formatBytes(bytes) {
  const units = [["GiB", BINARY.Gi], ["MiB", BINARY.Mi], ["KiB", BINARY.Ki]];
  for (const [name, size] of units) {
    if (bytes >= size) {
      return `${trim(bytes / size)} ${name}`;
    }
  }
  return `${trim(bytes, 0)} B`;
}

/** Plain-language readout shown under an input while it is being typed. */
export function describeQuantity(text, kind) {
  const {value, error} = parseQuantity(text);
  if (error || value === null) {
    return null;
  }
  return kind === "cpu" ? formatCores(value) : formatBytes(value);
}

export function coresToQuantity(cores) {
  return cores >= 1 ? trim(cores, 2) : `${Math.max(1, Math.round(cores * 1000))}m`;
}

export function bytesToQuantity(bytes) {
  if (bytes >= BINARY.Gi) {
    return `${trim(bytes / BINARY.Gi)}Gi`;
  }
  return `${Math.max(1, Math.ceil(bytes / BINARY.Mi))}Mi`;
}
