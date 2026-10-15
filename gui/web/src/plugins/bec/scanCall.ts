// Scan calls as a scientist would type them: scans.line_scan(dev.samx, -1, 1, steps=10).
// Port of the formatting half of bec_mcp_chat's scan_call.py.

const NAME = /^[A-Za-z_][A-Za-z0-9_]*$/;

function pyRepr(value: unknown, devices: Set<string>): string {
  if (typeof value === "string") return devices.has(value) ? `dev.${value}` : `'${value.replace(/\\/g, "\\\\").replace(/'/g, "\\'")}'`;
  if (value === null || value === undefined) return "None";
  if (typeof value === "boolean") return value ? "True" : "False";
  if (typeof value === "number") return Number.isInteger(value) ? String(value) : String(value);
  if (Array.isArray(value)) return `[${value.map((item) => pyRepr(item, devices)).join(", ")}]`;
  if (typeof value === "object") {
    const inner = Object.entries(value as Record<string, unknown>)
      .map(([key, item]) => `'${key}': ${pyRepr(item, new Set())}`)
      .join(", ");
    return `{${inner}}`;
  }
  return String(value);
}

function parseMaybeJson(value: unknown): unknown {
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}

/**
 * The call text for bec_run_scan's arguments ({scan | scan_name, args, kwargs}). String
 * positional arguments that look like names are taken to be devices, as scan arguments
 * almost always are.
 */
export function formatScanCall(input: Record<string, unknown>): string {
  const name = String(input.scan_name ?? input.scan ?? "?");
  const rawArgs = parseMaybeJson(input.args);
  const rawKwargs = parseMaybeJson(input.kwargs);
  const args = Array.isArray(rawArgs) ? rawArgs : [];
  const kwargs = rawKwargs && typeof rawKwargs === "object" && !Array.isArray(rawKwargs) ? (rawKwargs as Record<string, unknown>) : {};
  const devices = new Set(args.filter((arg): arg is string => typeof arg === "string" && NAME.test(arg)));
  const parts = [
    ...args.map((arg) => pyRepr(arg, devices)),
    ...Object.entries(kwargs).map(([key, value]) => `${key}=${pyRepr(value, new Set())}`),
  ];
  return `scans.${name}(${parts.join(", ")})`;
}

/** A scan call the user typed could not be read. */
export class ScanCallError extends Error {}

/** A device argument, written dev.<name> or dev["name"]. */
export class DeviceRef {
  constructor(readonly name: string) {}
}

export interface ParsedScanCall {
  scanName: string;
  args: unknown[];
  kwargs: Record<string, unknown>;
}

type Token = { kind: "name" | "number" | "string" | "punct"; value: string; pos: number };

function tokenize(source: string): Token[] {
  const tokens: Token[] = [];
  let i = 0;
  while (i < source.length) {
    const c = source[i];
    if (/\s/.test(c)) {
      i++;
    } else if (/[A-Za-z_]/.test(c)) {
      const m = /^[A-Za-z_][A-Za-z0-9_]*/.exec(source.slice(i))!;
      tokens.push({ kind: "name", value: m[0], pos: i });
      i += m[0].length;
    } else if (/[0-9]/.test(c) || (c === "." && /[0-9]/.test(source[i + 1] ?? ""))) {
      const m = /^(?:\d[\d_]*\.?[\d_]*|\.\d[\d_]*)(?:[eE][+-]?\d+)?/.exec(source.slice(i))!;
      tokens.push({ kind: "number", value: m[0].replace(/_/g, ""), pos: i });
      i += m[0].length;
    } else if (c === "'" || c === '"') {
      let j = i + 1;
      let value = "";
      while (j < source.length && source[j] !== c) {
        if (source[j] === "\\" && j + 1 < source.length) {
          const next = source[j + 1];
          value += next === "n" ? "\n" : next === "t" ? "\t" : next;
          j += 2;
        } else {
          value += source[j++];
        }
      }
      if (j >= source.length) throw new ScanCallError("a string is not closed");
      tokens.push({ kind: "string", value, pos: i });
      i = j + 1;
    } else if ("()[]{},:=.-+".includes(c)) {
      tokens.push({ kind: "punct", value: c, pos: i });
      i++;
    } else {
      throw new ScanCallError(`unexpected “${c}”`);
    }
  }
  return tokens;
}

/**
 * Reads scans.<name>(...) as typed in the BEC client. Only literals (numbers, strings, True,
 * False, None, lists, tuples, dicts) and dev.<name> references are accepted, so nothing typed
 * here is ever evaluated. Port of bec_mcp_chat's parse_scan_call.
 */
export function parseScanCall(text: string): ParsedScanCall {
  const tokens = tokenize(text.trim().replace(/;$/, ""));
  let at = 0;
  const peek = () => tokens[at];
  const take = (value?: string) => {
    const token = tokens[at];
    if (!token || (value !== undefined && token.value !== value)) {
      throw new ScanCallError(value ? `expected “${value}”${token ? ` near “${token.value}”` : " at the end"}` : "the call ends early");
    }
    at++;
    return token;
  };
  if (!tokens.length) throw new ScanCallError("the scan call is empty");

  // scans.name, bec.scans.name or name
  let name = take().value;
  if (name === "bec" && peek()?.value === ".") {
    take(".");
    name = take().value;
  }
  if (name === "scans" && peek()?.value === ".") {
    take(".");
    name = take().value;
  } else if (peek()?.value === ".") {
    throw new ScanCallError(`expected scans.<name>(...), got ${name}.…`);
  }
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) throw new ScanCallError("expected a scan name");

  const value = (): unknown => {
    const token = take();
    if (token.kind === "number") return Number(token.value);
    if (token.kind === "string") return token.value;
    if (token.value === "-" || token.value === "+") {
      const number = take();
      if (number.kind !== "number") throw new ScanCallError(`expected a number after “${token.value}”`);
      return token.value === "-" ? -Number(number.value) : Number(number.value);
    }
    if (token.kind === "name") {
      if (token.value === "True") return true;
      if (token.value === "False") return false;
      if (token.value === "None") return null;
      if (token.value === "dev") {
        if (peek()?.value === ".") {
          take(".");
          return new DeviceRef(take().value);
        }
        if (peek()?.value === "[") {
          take("[");
          const key = take();
          if (key.kind !== "string") throw new ScanCallError('write dev["name"] with a quoted name');
          take("]");
          return new DeviceRef(key.value);
        }
      }
      throw new ScanCallError(`only literals and dev.<name> are allowed, not “${token.value}”`);
    }
    if (token.value === "[" || token.value === "(") {
      const close = token.value === "[" ? "]" : ")";
      const items: unknown[] = [];
      while (peek()?.value !== close) {
        items.push(value());
        if (peek()?.value !== close) take(",");
      }
      take(close);
      return items;
    }
    if (token.value === "{") {
      const out: Record<string, unknown> = {};
      while (peek()?.value !== "}") {
        const key = value();
        take(":");
        out[String(key instanceof DeviceRef ? key.name : key)] = value();
        if (peek()?.value !== "}") take(",");
      }
      take("}");
      return out;
    }
    throw new ScanCallError(`unexpected “${token.value}”`);
  };

  take("(");
  const args: unknown[] = [];
  const kwargs: Record<string, unknown> = {};
  while (peek()?.value !== ")") {
    const token = peek();
    if (token?.kind === "name" && tokens[at + 1]?.value === "=") {
      at += 2;
      kwargs[token.value] = value();
    } else {
      if (Object.keys(kwargs).length) throw new ScanCallError("positional arguments must come before keyword arguments");
      args.push(value());
    }
    if (peek()?.value !== ")") take(",");
  }
  take(")");
  if (at !== tokens.length) throw new ScanCallError(`unexpected “${tokens[at].value}” after the call`);
  return { scanName: name, args, kwargs };
}

function plain(value: unknown): unknown {
  if (value instanceof DeviceRef) return value.name;
  if (Array.isArray(value)) return value.map(plain);
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [k, plain(v)]));
  }
  return value;
}

/** Arguments for bec_run_scan, with devices as plain names. */
export function scanToolArgs(call: ParsedScanCall): { scan: string; args: unknown[]; kwargs: Record<string, unknown> } {
  return { scan: call.scanName, args: plain(call.args) as unknown[], kwargs: plain(call.kwargs) as Record<string, unknown> };
}
