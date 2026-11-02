// A small in-memory stand-in for the Apps Script services Code.gs uses, so it can run under node:test.
// Only what Code.gs calls is implemented; a missing method fails the test loudly.

import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const CODE_PATH = path.join(here, '..', 'Code.gs');

class Range {
  constructor(sheet, row, col, numRows, numCols) {
    Object.assign(this, { sheet, row, col, numRows, numCols });
  }
  getValues() {
    const out = [];
    for (let r = 0; r < this.numRows; r++) {
      const line = this.sheet.data[this.row - 1 + r] || [];
      const vals = [];
      for (let c = 0; c < this.numCols; c++) {
        const v = line[this.col - 1 + c];
        vals.push(v === undefined ? '' : v);
      }
      out.push(vals);
    }
    return out;
  }
  setValues(values) {
    if (values.length !== this.numRows || values.some((v) => v.length !== this.numCols)) {
      throw new Error(`setValues: got ${values.length}x${values[0] && values[0].length}, range is ${this.numRows}x${this.numCols}`);
    }
    values.forEach((vals, r) => vals.forEach((v, c) => this.sheet.put(this.row + r, this.col + c, v)));
    this.sheet.writes.push({ row: this.row, numRows: this.numRows });
    return this;
  }
  setValue(v) {
    this.sheet.put(this.row, this.col, v);
    this.sheet.writes.push({ row: this.row, numRows: 1 });
    return this;
  }
  setNumberFormat() { return this; }
  clearContent() {
    for (let r = 0; r < this.numRows; r++) {
      for (let c = 0; c < this.numCols; c++) this.sheet.put(this.row + r, this.col + c, '');
    }
    return this;
  }
}

class Sheet {
  constructor(name, data) {
    this.name = name;
    this.data = (data || []).map((r) => r.slice());
    this.writes = []; // { row, numRows } of every write, to check what was touched
  }
  put(row, col, v) {
    while (this.data.length < row) this.data.push([]);
    const line = this.data[row - 1];
    while (line.length < col) line.push('');
    line[col - 1] = v;
  }
  getLastRow() {
    for (let r = this.data.length; r > 0; r--) {
      if (this.data[r - 1].some((v) => v !== '' && v !== undefined)) return r;
    }
    return 0;
  }
  getLastColumn() {
    let last = 0;
    this.data.forEach((line) => line.forEach((v, c) => { if (v !== '' && v !== undefined) last = Math.max(last, c + 1); }));
    return last;
  }
  getRange(row, col, numRows = 1, numCols = 1) {
    return new Range(this, row, col, numRows, numCols);
  }
  getDataRange() {
    return new Range(this, 1, 1, Math.max(1, this.getLastRow()), Math.max(1, this.getLastColumn()));
  }
  // Rows (header excluded) as objects keyed by header, for assertions.
  objects() {
    const [header, ...rows] = this.data;
    return rows
      .filter((r) => r.some((v) => v !== '' && v !== undefined))
      .map((r) => Object.fromEntries(header.map((h, i) => [h, r[i] === undefined ? '' : r[i]])));
  }
}

class Spreadsheet {
  constructor(tabs) {
    this.sheets = {};
    Object.entries(tabs || {}).forEach(([name, data]) => { this.sheets[name] = new Sheet(name, data); });
  }
  getSheetByName(name) { return this.sheets[name] || null; }
  insertSheet(name) {
    if (this.sheets[name]) throw new Error('sheet exists: ' + name);
    this.sheets[name] = new Sheet(name, []);
    return this.sheets[name];
  }
}

const pad = (n) => String(n).padStart(2, '0');

// Asia/Jakarta is UTC+7 all year (no DST), which is all Code.gs formats in.
function formatDate(date, tz, fmt) {
  if (tz !== 'Asia/Jakarta') throw new Error('mock formatDate only knows Asia/Jakarta');
  const d = new Date(date.getTime() + 7 * 3600000);
  const ymd = `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`;
  if (fmt === 'yyyy-MM-dd') return ymd;
  if (fmt === "yyyy-MM-dd'T'HH:mm:ssXXX") {
    return `${ymd}T${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}+07:00`;
  }
  throw new Error('mock formatDate: unsupported format ' + fmt);
}

// Loads Code.gs into a fresh context over `tabs` ({ name: [[header...], [row...]] }).
export function loadCode(tabs, { properties = { READ_TOKEN: 'r', WRITE_TOKEN: 'w' } } = {}) {
  const ss = new Spreadsheet(tabs);
  const cache = new Map();
  const logs = [];
  const ctx = {
    SpreadsheetApp: { getActiveSpreadsheet: () => ss },
    PropertiesService: { getScriptProperties: () => ({ getProperty: (k) => (k in properties ? properties[k] : null) }) },
    CacheService: {
      getScriptCache: () => ({
        get: (k) => (cache.has(k) ? cache.get(k) : null),
        put: (k, v) => { cache.set(k, v); },
        remove: (k) => { cache.delete(k); },
      }),
    },
    LockService: { getScriptLock: () => ({ tryLock: () => true, releaseLock: () => {} }) },
    Utilities: { formatDate },
    ContentService: {
      MimeType: { JSON: 'application/json' },
      createTextOutput: (content) => ({ content, setMimeType() { return this; } }),
    },
    Logger: { log: (s) => logs.push(s) },
    Date, JSON, Math, Object, Array, String, Number, isNaN, parseInt, Infinity,
  };
  vm.createContext(ctx);
  vm.runInContext(fs.readFileSync(CODE_PATH, 'utf8'), ctx, { filename: 'Code.gs' });

  const call = (fn, e) => JSON.parse(fn(e).content);
  return {
    ss,
    cache,
    logs,
    ctx,
    get: (params) => call(ctx.doGet, { parameter: params }),
    post: (mode, body, token = 'w') => call(ctx.doPost, {
      parameter: { token, mode },
      postData: { contents: JSON.stringify(body) },
    }),
  };
}
