/**
 * MakanDiMana — "Get Data" for the MVP, running on Google Apps Script.
 *
 * Reads three tabs (brands, outlets, promos) from the spreadsheet this script is bound to
 * and serves the feed as JSON.
 *
 *   GET {WEB_APP_URL}?token=READ&mode=swipe&cells=0:88658c1a9bfffff,1:88658c1a9dfffff&offset=0&limit=10
 *   GET {WEB_APP_URL}?token=READ&mode=nearby&cells=...
 *   GET {WEB_APP_URL}?token=WRITE&mode=brands                (places fetcher reads its brand list)
 *   POST {WEB_APP_URL}?token=WRITE&mode=upsert_outlets       (places fetcher writes the outlets tab)
 *
 * Promo fetcher (all WRITE_TOKEN; tabs `sources`, `raw_items`, `promos`):
 *   GET  {WEB_APP_URL}?token=WRITE&mode=sources                   every source row
 *   GET  {WEB_APP_URL}?token=WRITE&mode=raw_items&status=failed   raw items with that status
 *   GET  {WEB_APP_URL}?token=WRITE&mode=raw_item_refs             stored refs per source, any status
 *   POST {WEB_APP_URL}?token=WRITE&mode=update_sources            fetcher status columns only
 *   POST {WEB_APP_URL}?token=WRITE&mode=upsert_raw_items          by id; prunes rows past 90 days
 *   POST {WEB_APP_URL}?token=WRITE&mode=upsert_promos             by id; source = web rows only
 * Run setupPromoTabs() once from the editor to add the tabs, the new promos columns and the seed sources.
 *
 * `cells` is "ring:h3cell" pairs computed on the phone with h3-js (gridDisk with distances),
 * so this script never needs an H3 library and never sees coordinates.
 *
 * Two tokens, so a leaked feed token can't change the sheet:
 *   READ_TOKEN   feed reads; held by the Next.js server (server-only env var, never in the browser)
 *   WRITE_TOKEN  fetcher calls; held by GitHub Actions secrets (SHEET_TOKEN)
 * A call whose token property isn't set is refused.
 *
 * Setup: Project Settings → Script properties → add READ_TOKEN and WRITE_TOKEN (two different
 * long random values), then delete the old TOKEN property.
 * Deploy: Deploy → New deployment → Web app → Execute as: Me → Who has access: Anyone.
 * Keep the spreadsheet itself shared as Restricted; the web app reads it as you.
 */

const TZ = 'Asia/Jakarta';
const CACHE_SECONDS = 300;      // re-read the sheet at most every 5 minutes
const MAX_LIMIT = 50;

function doGet(e) {
  const p = (e && e.parameter) || {};

  if (p.mode === 'brands') {
    if (!authorized_(p, 'WRITE_TOKEN')) return json_({ ok: false, error: 'unauthorized' });
    return json_({ ok: true, brands: readTab_('brands').map(function (b) {
      return { id: String(b.id), name: String(b.name), aliases: String(b.aliases || '') };
    }) });
  }

  if (p.mode === 'sources' || p.mode === 'raw_items' || p.mode === 'raw_item_refs') {
    if (!authorized_(p, 'WRITE_TOKEN')) return json_({ ok: false, error: 'unauthorized' });
    try {
      if (p.mode === 'sources') return json_({ ok: true, sources: getSources_() });
      if (p.mode === 'raw_item_refs') return json_({ ok: true, refs: getRawItemRefs_() });
      return json_({ ok: true, raw_items: getRawItems_(String(p.status || '')) });
    } catch (err) {
      return json_({ ok: false, error: String(err && err.message || err) });
    }
  }

  if (!authorized_(p, 'READ_TOKEN')) return json_({ ok: false, error: 'unauthorized' });

  const mode = p.mode === 'nearby' ? 'nearby' : 'swipe';
  const ringOf = parseCells_(p.cells);
  const offset = Math.max(0, parseInt(p.offset, 10) || 0);
  const limit = Math.min(MAX_LIMIT, Math.max(1, parseInt(p.limit, 10) || 10));

  try {
    const data = loadData_();
    const result = mode === 'nearby'
      ? nearby_(data, ringOf)
      : swipe_(data, ringOf, offset, limit);
    return json_(Object.assign({ ok: true, mode: mode }, result));
  } catch (err) {
    return json_({ ok: false, error: String(err && err.message || err) });
  }
}

// True when the request's token matches the named script property; an unset property refuses.
function authorized_(p, property) {
  const expected = PropertiesService.getScriptProperties().getProperty(property);
  return !!expected && p.token === expected;
}

/* ---------- places fetcher writes ---------- */

const OUTLET_FIELDS = ['id', 'brand_id', 'name', 'address', 'lat', 'lng', 'h3_r8', 'h3_r6', 'format', 'mall_name', 'source'];
const CLOSE_AFTER_DAYS = 25;   // missing from two twice-a-month runs → closed

// Each write mode reads the parsed body and returns its counts; `feed` modes also clear the feed cache.
const WRITE_MODES = {
  upsert_outlets: { feed: true, run: function (body) {
    if (!body.source || !Array.isArray(body.outlets)) throw new Error('need source and outlets');
    return upsertOutlets_(String(body.source), body.outlets);
  } },
  update_sources: { feed: false, run: function (body) {
    if (!Array.isArray(body.sources)) throw new Error('need sources');
    return updateSources_(body.sources);
  } },
  upsert_raw_items: { feed: false, run: function (body) {
    if (!Array.isArray(body.raw_items)) throw new Error('need raw_items');
    return upsertRawItems_(body.raw_items);
  } },
  upsert_promos: { feed: true, run: function (body) {
    if (!Array.isArray(body.promos)) throw new Error('need promos');
    return upsertPromos_(body.promos);
  } }
};

function doPost(e) {
  const p = (e && e.parameter) || {};
  if (!authorized_(p, 'WRITE_TOKEN')) return json_({ ok: false, error: 'unauthorized' });
  const mode = WRITE_MODES.hasOwnProperty(p.mode) ? WRITE_MODES[p.mode] : null;
  if (!mode) return json_({ ok: false, error: 'unknown mode' });

  const lock = LockService.getScriptLock();
  if (!lock.tryLock(30000)) return json_({ ok: false, error: 'busy, retry later' });
  try {
    const body = JSON.parse(e.postData.contents);
    const result = mode.run(body);
    if (mode.feed) CacheService.getScriptCache().remove('mdm_data');
    return json_(Object.assign({ ok: true }, result));
  } catch (err) {
    return json_({ ok: false, error: String(err && err.message || err) });
  } finally {
    lock.releaseLock();
  }
}

// Updates rows by id, appends new ones, and closes this source's rows not seen for CLOSE_AFTER_DAYS.
// Rows from other sources (manual) and extra columns are left alone; status on a seen row becomes open.
function upsertOutlets_(source, incoming) {
  const sheet = SpreadsheetApp.getActiveSpreadsheet().getSheetByName('outlets');
  if (!sheet) throw new Error('Missing tab: outlets');
  let header = sheet.getRange(1, 1, 1, sheet.getLastColumn()).getValues()[0].map(function (h) { return String(h).trim(); });
  OUTLET_FIELDS.concat(['status', 'last_seen_at']).forEach(function (f) {
    if (header.indexOf(f) === -1) { header.push(f); sheet.getRange(1, header.length).setValue(f); }
  });
  const col = {};
  header.forEach(function (h, i) { col[h] = i; });

  const rows = sheet.getLastRow() > 1
    ? sheet.getRange(2, 1, sheet.getLastRow() - 1, header.length).getValues()
    : [];
  const rowById = {};
  rows.forEach(function (r, i) { if (String(r[col.id]).trim()) rowById[String(r[col.id]).trim()] = i; });

  const now = new Date();
  const today = Utilities.formatDate(now, TZ, 'yyyy-MM-dd');
  const cutoff = Utilities.formatDate(new Date(now.getTime() - CLOSE_AFTER_DAYS * 86400000), TZ, 'yyyy-MM-dd');
  const seen = {};
  let added = 0, updated = 0, closed = 0;

  incoming.forEach(function (o) {
    const id = String(o.id || '').trim();
    if (!id) return;
    seen[id] = true;
    let row;
    if (id in rowById) { row = rows[rowById[id]]; updated++; }
    else { row = header.map(function () { return ''; }); rows.push(row); rowById[id] = rows.length - 1; added++; }
    OUTLET_FIELDS.forEach(function (f) { row[col[f]] = o[f] === undefined ? '' : o[f]; });
    row[col.source] = source;
    row[col.status] = 'open';
    row[col.last_seen_at] = today;
  });

  rows.forEach(function (row) {
    if (String(row[col.source]) !== source || seen[String(row[col.id]).trim()]) return;
    if (String(row[col.status]) === 'closed') return;
    if (dateStr_(row[col.last_seen_at]) < cutoff) { row[col.status] = 'closed'; closed++; }
  });

  if (rows.length) {
    // Plain text for ids and h3 cells so Sheets never reinterprets them as numbers.
    ['id', 'h3_r8', 'h3_r6'].forEach(function (f) { sheet.getRange(2, col[f] + 1, rows.length, 1).setNumberFormat('@'); });
    sheet.getRange(2, 1, rows.length, header.length).setValues(rows);
  }
  return { added: added, updated: updated, closed: closed };
}

/* ---------- promo fetcher ---------- */

// sources: one row per page the promo fetcher reads. You own the setup columns; the fetcher only
// writes SOURCE_FETCHER_FIELDS, and status only from active to error.
const SOURCE_FIELDS = ['id', 'type', 'target', 'brand_id', 'is_aggregator', 'fetch_every_hours', 'status',
  'last_fetched_at', 'last_seen_ref', 'error_count', 'last_error', 'notes'];
const SOURCE_FETCHER_FIELDS = ['last_fetched_at', 'last_seen_ref', 'error_count', 'last_error'];

// raw_items: every post or page read, so a run can skip what it has seen and re-parse without re-fetching.
const RAW_ITEM_FIELDS = ['id', 'source_id', 'ref', 'url', 'posted_at', 'text', 'status', 'attempts', 'error',
  'promo_ids', 'fetched_at'];
const RAW_ITEM_STATUSES = ['new', 'parsed', 'skipped', 'failed'];
const RAW_TEXT_MAX = 2000;
const RAW_KEEP_DAYS = 90;

// promos: what the fetcher writes on a source = web row; source, first_seen_at and last_seen_at are set here.
const PROMO_FIELDS = ['id', 'brand_id', 'brand_text', 'title', 'deal', 'channel', 'valid_from', 'valid_until',
  'posted_at', 'outlet_ids', 'status', 'action_url', 'source_ids', 'source_refs', 'dedupe_key', 'confidence'];
const PROMO_SOURCE = 'web';

// Launch sources from the promo fetcher RFC; setupPromoTabs() writes them only into an empty sources tab.
const SEED_SOURCES = [
  ['src_bca_fnb', 'web_page', 'https://promo.bca.co.id/id/all/c/fnb', '', 1, 24, 'active', 'BCA F&B promo page'],
  ['src_ramenya_katalog', 'web_page', 'https://katalogpromosi.com/restoran-dan-cafe/ramenya/', 'brand_ramenya', 0, 24, 'active', ''],
  ['src_ramenya_giladiskon', 'web_page', 'https://www.giladiskon.com/merchant/ramen-ya', 'brand_ramenya', 0, 24, 'active', ''],
  ['src_ramenya_qpon', 'web_page', 'https://qpon.id/', 'brand_ramenya', 0, 24, 'paused', 'Needs headless Chrome and the RamenYa brand page URL'],
  ['src_hokben_katalog', 'web_page', 'https://katalogpromosi.com/restoran-dan-cafe/hokben/', 'brand_hokben', 0, 24, 'active', ''],
  ['src_richeese_katalog', 'web_page', 'https://katalogpromosi.com/restoran-dan-cafe/richeese-factory/', 'brand_richeese', 0, 24, 'paused', 'Activate once brand_richeese is in brands'],
  ['src_hokben_giladiskon', 'web_page', 'https://www.giladiskon.com/merchant/hokben', 'brand_hokben', 0, 24, 'active', ''],
  ['src_marugame_giladiskon', 'web_page', 'https://www.giladiskon.com/merchant/marugame-udon', 'brand_marugame', 0, 24, 'active', ''],
  ['src_chatime_giladiskon', 'web_page', 'https://www.giladiskon.com/merchant/chatime', 'brand_chatime', 0, 24, 'paused', 'Activate once brand_chatime is in brands']
];

function getSources_() {
  return readTab_('sources').map(function (s) {
    return {
      id: String(s.id).trim(),
      type: String(s.type || '').trim(),
      target: String(s.target || '').trim(),
      brand_id: String(s.brand_id || '').trim(),
      is_aggregator: Number(s.is_aggregator) === 1,
      fetch_every_hours: Number(s.fetch_every_hours) || 24,
      status: String(s.status || 'active').trim(),
      last_fetched_at: timeStr_(s.last_fetched_at),
      last_seen_ref: String(s.last_seen_ref || ''),
      error_count: Number(s.error_count) || 0,
      last_error: String(s.last_error || ''),
      notes: String(s.notes || '')
    };
  });
}

// Writes only the fetcher's cells of rows that exist; your setup columns and unknown ids are left alone.
function updateSources_(incoming) {
  const t = openTab_('sources', SOURCE_FIELDS);
  let updated = 0, skipped = 0;
  incoming.forEach(function (s) {
    const i = t.rowById[String(s.id || '').trim()];
    if (i === undefined) { skipped++; return; }
    const row = t.rows[i];
    SOURCE_FETCHER_FIELDS.forEach(function (f) {
      if (s[f] === undefined) return;
      row[t.col[f]] = s[f];
      setCell_(t, i, f, s[f]);
    });
    if (s.status === 'error' && String(row[t.col.status]).trim() === 'active') {
      row[t.col.status] = 'error';
      setCell_(t, i, 'status', 'error');
    }
    updated++;
  });
  return { updated: updated, skipped: skipped };
}

function getRawItems_(status) {
  if (RAW_ITEM_STATUSES.indexOf(status) === -1) throw new Error('status must be one of ' + RAW_ITEM_STATUSES.join(', '));
  return readTab_('raw_items')
    .filter(function (r) { return String(r.status).trim() === status; })
    .map(function (r) {
      const obj = {};
      RAW_ITEM_FIELDS.forEach(function (f) { obj[f] = r[f] === undefined ? '' : r[f]; });
      obj.posted_at = timeStr_(r.posted_at);
      obj.fetched_at = timeStr_(r.fetched_at);
      obj.attempts = Number(r.attempts) || 0;
      return obj;
    });
}

// { source_id: [ref, ...] } for every stored raw item, so an adapter only opens items it hasn't read.
function getRawItemRefs_() {
  const refs = {};
  readTab_('raw_items').forEach(function (r) {
    const src = String(r.source_id || '').trim();
    const ref = String(r.ref || '').trim();
    if (!src || !ref) return;
    (refs[src] = refs[src] || []).push(ref);
  });
  return refs;
}

// Adds or updates by id, then drops rows fetched more than RAW_KEEP_DAYS ago. The tab is the fetcher's,
// so it is rewritten whole.
function upsertRawItems_(incoming) {
  const t = openTab_('raw_items', RAW_ITEM_FIELDS);
  let added = 0, updated = 0;
  incoming.forEach(function (r) {
    const id = String(r.id || '').trim();
    if (!id || !String(r.source_id || '').trim()) throw new Error('raw item needs id and source_id');
    if (r.status !== undefined && RAW_ITEM_STATUSES.indexOf(r.status) === -1) throw new Error('bad status for ' + id + ': ' + r.status);
    let row;
    if (id in t.rowById) { row = t.rows[t.rowById[id]]; updated++; }
    else { row = blankRow_(t); t.rows.push(row); t.rowById[id] = t.rows.length - 1; added++; }
    RAW_ITEM_FIELDS.forEach(function (f) {
      if (r[f] === undefined) return;
      let v = Array.isArray(r[f]) ? r[f].join(',') : r[f];
      if (f === 'text') v = String(v).slice(0, RAW_TEXT_MAX);
      row[t.col[f]] = v;
    });
    if (!String(row[t.col.status]).trim()) row[t.col.status] = 'new';
    if (!String(row[t.col.fetched_at]).trim()) row[t.col.fetched_at] = nowStr_();
  });

  const cutoff = new Date(Date.now() - RAW_KEEP_DAYS * 86400000);
  const kept = t.rows.filter(function (row) {
    const at = new Date(timeStr_(row[t.col.fetched_at]));
    return isNaN(at.getTime()) || at >= cutoff;
  });
  const pruned = t.rows.length - kept.length;

  if (t.rows.length) t.sheet.getRange(2, 1, t.rows.length, t.header.length).clearContent();
  if (kept.length) {
    ['id', 'ref', 'posted_at', 'fetched_at'].forEach(function (f) {
      t.sheet.getRange(2, t.col[f] + 1, kept.length, 1).setNumberFormat('@');
    });
    t.sheet.getRange(2, 1, kept.length, t.header.length).setValues(kept);
  }
  return { added: added, updated: updated, pruned: pruned };
}

// Upserts by id. Only rows with source = web are the fetcher's: any other existing row (manual) is
// skipped and never rewritten. status is whatever the fetcher computed, so a hand-hidden web promo
// can come back; set its source to manual to keep it out.
function upsertPromos_(incoming) {
  const t = openTab_('promos', PROMO_FIELDS.concat(['source', 'first_seen_at', 'last_seen_at']));
  const today = Utilities.formatDate(new Date(), TZ, 'yyyy-MM-dd');
  let added = 0, updated = 0, skipped = 0;
  const appended = [];

  incoming.forEach(function (pr) {
    const id = String(pr.id || '').trim();
    if (!/^promo_[a-z0-9_]+$/.test(id)) throw new Error('promo id must look like promo_<a-z0-9_>: ' + id);
    let row, i = t.rowById[id];
    if (i !== undefined) {
      row = t.rows[i];
      if (String(row[t.col.source]).trim() !== PROMO_SOURCE) { skipped++; return; }
      updated++;
    } else {
      row = blankRow_(t);
      row[t.col.first_seen_at] = today;
      t.rows.push(row); i = t.rows.length - 1; t.rowById[id] = i;
      appended.push(i);
      added++;
    }
    PROMO_FIELDS.forEach(function (f) {
      if (pr[f] === undefined) return;
      row[t.col[f]] = Array.isArray(pr[f]) ? pr[f].join(',') : pr[f];
    });
    row[t.col.source] = PROMO_SOURCE;
    row[t.col.last_seen_at] = today;
    if (appended.indexOf(i) === -1) setRow_(t, i);
  });

  if (appended.length) {
    const first = appended[0];
    t.sheet.getRange(first + 2, t.col.id + 1, appended.length, 1).setNumberFormat('@');
    t.sheet.getRange(first + 2, 1, appended.length, t.header.length).setValues(t.rows.slice(first));
  }
  return { added: added, updated: updated, skipped: skipped };
}

/* Run once from the editor: adds the sources and raw_items tabs, the new promos columns, and the
 * seed sources (only into an empty sources tab). Safe to run again; it never removes or renames anything. */
function setupPromoTabs() {
  const ss = SpreadsheetApp.getActiveSpreadsheet();
  [['sources', SOURCE_FIELDS], ['raw_items', RAW_ITEM_FIELDS]].forEach(function (tab) {
    if (!ss.getSheetByName(tab[0])) ss.insertSheet(tab[0]).getRange(1, 1, 1, tab[1].length).setValues([tab[1]]);
  });
  openTab_('promos', PROMO_FIELDS.concat(['source', 'first_seen_at', 'last_seen_at']));

  const t = openTab_('sources', SOURCE_FIELDS);
  let seeded = 0;
  if (!t.rows.length) {
    const rows = SEED_SOURCES.map(function (s) {
      const row = blankRow_(t);
      ['id', 'type', 'target', 'brand_id', 'is_aggregator', 'fetch_every_hours', 'status', 'notes']
        .forEach(function (f, k) { row[t.col[f]] = s[k]; });
      row[t.col.error_count] = 0;
      return row;
    });
    t.sheet.getRange(2, 1, rows.length, t.header.length).setValues(rows);
    seeded = rows.length;
  }
  const result = { seeded_sources: seeded };
  Logger.log(JSON.stringify(result));
  return result;
}

// Opens a tab, appends any missing column in `fields` to its header, and reads its data rows.
function openTab_(name, fields) {
  const sheet = SpreadsheetApp.getActiveSpreadsheet().getSheetByName(name);
  if (!sheet) throw new Error('Missing tab: ' + name + ' (run setupPromoTabs)');
  const width = Math.max(1, sheet.getLastColumn());
  const header = sheet.getRange(1, 1, 1, width).getValues()[0].map(function (h) { return String(h).trim(); });
  while (header.length && header[header.length - 1] === '') header.pop();
  fields.forEach(function (f) {
    if (header.indexOf(f) === -1) { header.push(f); sheet.getRange(1, header.length).setValue(f); }
  });
  const col = {};
  header.forEach(function (h, i) { col[h] = i; });
  const rows = sheet.getLastRow() > 1
    ? sheet.getRange(2, 1, sheet.getLastRow() - 1, header.length).getValues()
    : [];
  const rowById = {};
  rows.forEach(function (r, i) { const id = String(r[col.id]).trim(); if (id) rowById[id] = i; });
  return { sheet: sheet, header: header, col: col, rows: rows, rowById: rowById };
}

function blankRow_(t) {
  return t.header.map(function () { return ''; });
}

// Writes one existing row (index into t.rows) back to the sheet.
function setRow_(t, i) {
  t.sheet.getRange(i + 2, 1, 1, t.header.length).setValues([t.rows[i]]);
}

function setCell_(t, i, field, value) {
  const cell = t.sheet.getRange(i + 2, t.col[field] + 1);
  if (field === 'last_fetched_at' || field === 'last_seen_ref') cell.setNumberFormat('@');
  cell.setValue(value);
}

function nowStr_() {
  return Utilities.formatDate(new Date(), TZ, "yyyy-MM-dd'T'HH:mm:ssXXX");
}

// A timestamp cell as an ISO string; Sheets may have turned a written string into a Date.
function timeStr_(v) {
  if (!v) return '';
  if (Object.prototype.toString.call(v) === '[object Date]') return Utilities.formatDate(v, TZ, "yyyy-MM-dd'T'HH:mm:ssXXX");
  return String(v);
}

/* ---------- feed modes ---------- */

// One card per promo: nearest outlet ring first, then newest post. Promos with no outlet in range follow.
function swipe_(data, ringOf, offset, limit) {
  const cards = data.promos.map(function (pr) {
    const near = nearestOutlet_(data, pr, ringOf);
    return { promo: pr, outlet: near ? near.outlet : null, ring: near ? near.ring : null };
  });
  cards.sort(function (a, b) {
    const ra = a.ring === null ? Infinity : a.ring;
    const rb = b.ring === null ? Infinity : b.ring;
    if (ra !== rb) return ra - rb;
    return (b.promo.posted_at || '').localeCompare(a.promo.posted_at || '');
  });
  const page = cards.slice(offset, offset + limit);
  const nextOffset = offset + limit < cards.length ? offset + limit : 0; // wrap around
  return {
    total: cards.length,
    next_offset: nextOffset,
    items: page.map(function (c) { return card_(c.promo, c.outlet, c.ring, data); })
  };
}

// One row per outlet inside the rings that has an active promo, nearest ring first.
function nearby_(data, ringOf) {
  if (!Object.keys(ringOf).length) return { items: [], needs_location: true };
  const rows = [];
  data.outlets.forEach(function (o) {
    if (!(o.h3_r8 in ringOf) || o.status !== 'open') return;
    const promos = data.promos.filter(function (pr) { return appliesTo_(pr, o); });
    if (!promos.length) return;
    promos.sort(function (a, b) { return (b.posted_at || '').localeCompare(a.posted_at || ''); });
    rows.push({ outlet: o, ring: ringOf[o.h3_r8], best: promos[0], others: promos.length - 1 });
  });
  rows.sort(function (a, b) { return a.ring - b.ring; });
  return {
    items: rows.map(function (r) {
      return Object.assign(card_(r.best, r.outlet, r.ring, data), { other_promos: r.others });
    })
  };
}

/* ---------- helpers ---------- */

function nearestOutlet_(data, promo, ringOf) {
  let best = null;
  data.outlets.forEach(function (o) {
    if (o.status !== 'open' || !(o.h3_r8 in ringOf) || !appliesTo_(promo, o)) return;
    const ring = ringOf[o.h3_r8];
    if (!best || ring < best.ring) best = { outlet: o, ring: ring };
  });
  return best;
}

// A promo applies to an outlet if it lists that outlet, or (when it lists none) to every outlet of its brand.
function appliesTo_(promo, outlet) {
  if (promo.outlet_ids.length) return promo.outlet_ids.indexOf(outlet.id) !== -1;
  return !!promo.brand_id && promo.brand_id === outlet.brand_id;
}

function card_(pr, o, ring, data) {
  const brand = data.brandsById[pr.brand_id];
  return {
    promo_id: pr.id,
    brand: brand ? brand.name : (pr.brand_text || ''),
    title: pr.title,
    deal: pr.deal,
    channel: pr.channel,
    valid_from: pr.valid_from,
    valid_until: pr.valid_until,
    action_url: pr.action_url,
    outlet: o ? { id: o.id, name: o.name, address: o.address, lat: o.lat, lng: o.lng } : null,
    ring: ring
  };
}

// "0:cellA,1:cellB,1:cellC" → { cellA: 0, cellB: 1, cellC: 1 }
function parseCells_(raw) {
  const map = {};
  String(raw || '').split(',').forEach(function (pair) {
    const parts = pair.split(':');
    if (parts.length !== 2) return;
    const ring = parseInt(parts[0], 10);
    const cell = parts[1].trim().toLowerCase();
    if (!isNaN(ring) && /^[0-9a-f]{15}$/.test(cell)) map[cell] = ring;
  });
  return map;
}

/* ---------- data loading with cache ---------- */

function loadData_() {
  const cache = CacheService.getScriptCache();
  const hit = cache.get('mdm_data');
  if (hit) return JSON.parse(hit);

  const today = Utilities.formatDate(new Date(), TZ, 'yyyy-MM-dd');
  const brands = readTab_('brands');
  const outlets = readTab_('outlets').map(function (o) {
    o.h3_r8 = String(o.h3_r8 || '').trim().toLowerCase();
    o.status = String(o.status || 'open').trim();
    return o;
  });
  const promos = readTab_('promos')
    .map(function (pr) {
      pr.valid_from = dateStr_(pr.valid_from);
      pr.valid_until = dateStr_(pr.valid_until);
      pr.posted_at = dateStr_(pr.posted_at);
      pr.outlet_ids = String(pr.outlet_ids || '').split(',').map(function (s) { return s.trim(); }).filter(String);
      return pr;
    })
    .filter(function (pr) {
      if (String(pr.status || 'active') !== 'active') return false;
      return !pr.valid_until || pr.valid_until >= today;   // keep current and upcoming promos
    });

  const brandsById = {};
  brands.forEach(function (b) { brandsById[b.id] = b; });
  const data = { brandsById: brandsById, outlets: outlets, promos: promos };

  const text = JSON.stringify(data);
  if (text.length < 95000) cache.put('mdm_data', text, CACHE_SECONDS); // cache values are capped at 100 KB
  return data;
}

// Reads a tab whose first row holds column names into an array of objects.
function readTab_(name) {
  const sheet = SpreadsheetApp.getActiveSpreadsheet().getSheetByName(name);
  if (!sheet) throw new Error('Missing tab: ' + name);
  const values = sheet.getDataRange().getValues();
  const header = values.shift().map(function (h) { return String(h).trim(); });
  return values
    .filter(function (row) { return String(row[0]).trim() !== ''; })
    .map(function (row) {
      const obj = {};
      header.forEach(function (h, i) { obj[h] = row[i]; });
      return obj;
    });
}

function dateStr_(v) {
  if (!v) return '';
  if (Object.prototype.toString.call(v) === '[object Date]') return Utilities.formatDate(v, TZ, 'yyyy-MM-dd');
  return String(v).slice(0, 10);
}

function json_(obj) {
  return ContentService.createTextOutput(JSON.stringify(obj)).setMimeType(ContentService.MimeType.JSON);
}

/* Run from the editor to check the tabs parse and the feed works. */
function testFeed() {
  const data = loadData_();
  const anyCell = data.outlets.length ? data.outlets[0].h3_r8 : '';
  Logger.log(JSON.stringify(swipe_(data, anyCell ? parseCells_('0:' + anyCell) : {}, 0, 5), null, 2));
}
