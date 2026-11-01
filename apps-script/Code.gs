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

function doPost(e) {
  const p = (e && e.parameter) || {};
  if (!authorized_(p, 'WRITE_TOKEN')) return json_({ ok: false, error: 'unauthorized' });
  if (p.mode !== 'upsert_outlets') return json_({ ok: false, error: 'unknown mode' });

  const lock = LockService.getScriptLock();
  if (!lock.tryLock(30000)) return json_({ ok: false, error: 'busy, retry later' });
  try {
    const body = JSON.parse(e.postData.contents);
    if (!body.source || !Array.isArray(body.outlets)) throw new Error('need source and outlets');
    const result = upsertOutlets_(String(body.source), body.outlets);
    CacheService.getScriptCache().remove('mdm_data');
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
