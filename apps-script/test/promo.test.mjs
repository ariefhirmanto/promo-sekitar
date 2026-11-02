// Promo fetcher endpoints in Code.gs (RFC M1), against a mocked sheet. Run: node --test 'apps-script/test/*.test.mjs'
import test from 'node:test';
import assert from 'node:assert/strict';
import { loadCode } from './gas-mock.mjs';

const SOURCE_HEADER = ['id', 'type', 'target', 'brand_id', 'is_aggregator', 'fetch_every_hours', 'status',
  'last_fetched_at', 'last_seen_ref', 'error_count', 'last_error', 'notes'];
const RAW_HEADER = ['id', 'source_id', 'ref', 'url', 'posted_at', 'text', 'status', 'attempts', 'error',
  'promo_ids', 'fetched_at'];
// The promos tab as it is today: no promo fetcher columns yet.
const PROMO_HEADER = ['id', 'brand_id', 'brand_text', 'title', 'deal', 'channel', 'valid_from', 'valid_until',
  'posted_at', 'outlet_ids', 'status', 'action_url', 'source'];

const iso = (msAgo) => new Date(Date.now() - msAgo).toISOString();
const DAY = 86400000;

function tabs(extra = {}) {
  return {
    brands: [['id', 'name', 'aliases'], ['brand_ramenya', 'Ramen Ya', 'RamenYa']],
    outlets: [['id', 'brand_id', 'name', 'status', 'h3_r8'], ['out_osm_n1', 'brand_ramenya', 'RamenYa Margo', 'open', '88658c1a9bfffff']],
    promos: [PROMO_HEADER,
      ['promo_hokben_bca28', 'brand_hokben', '', 'Diskon 28% dengan BCA', 'Cek syarat', 'dine_in', '2026-10-27', '2026-11-19', '2026-10-27', '', 'active', 'https://promo.bca.co.id/', 'manual']],
    sources: [SOURCE_HEADER,
      ['src_bca_fnb', 'web_page', 'https://promo.bca.co.id/id/all/c/fnb', '', 1, 24, 'active', '', '', 0, '', 'my note'],
      ['src_chatime_giladiskon', 'web_page', 'https://www.giladiskon.com/merchant/chatime', 'brand_chatime', 0, 24, 'paused', '', '', 0, '', '']],
    raw_items: [RAW_HEADER],
    ...extra,
  };
}

test('promo endpoints refuse the read token and a missing token', () => {
  const s = loadCode(tabs());
  assert.equal(s.get({ mode: 'sources', token: 'r' }).error, 'unauthorized');
  assert.equal(s.get({ mode: 'raw_items', status: 'new' }).error, 'unauthorized');
  for (const mode of ['update_sources', 'upsert_raw_items', 'upsert_promos']) {
    assert.equal(s.post(mode, {}, 'r').error, 'unauthorized', mode);
  }
  assert.equal(s.post('drop_everything', {}).error, 'unknown mode');
});

test('GET sources returns every row with typed fields', () => {
  const res = loadCode(tabs()).get({ mode: 'sources', token: 'w' });
  assert.equal(res.ok, true);
  assert.equal(res.sources.length, 2);
  assert.deepEqual(res.sources[0], {
    id: 'src_bca_fnb', type: 'web_page', target: 'https://promo.bca.co.id/id/all/c/fnb', brand_id: '',
    is_aggregator: true, fetch_every_hours: 24, status: 'active', last_fetched_at: '', last_seen_ref: '',
    error_count: 0, last_error: '', notes: 'my note',
  });
  assert.equal(res.sources[1].status, 'paused');
});

test('GET sources says to run setup when the tab is missing', () => {
  const t = tabs();
  delete t.sources;
  const res = loadCode(t).get({ mode: 'sources', token: 'w' });
  assert.equal(res.ok, false);
  assert.match(res.error, /Missing tab: sources/);
});

test('update_sources writes only the fetcher columns, and status only active → error', () => {
  const s = loadCode(tabs());
  const res = s.post('update_sources', { sources: [
    { id: 'src_bca_fnb', last_fetched_at: '2026-11-02T06:01:00+07:00', error_count: 3, last_error: 'HTTP 503',
      status: 'error', target: 'https://evil.example', notes: 'overwritten?' },
    { id: 'src_chatime_giladiskon', status: 'error', error_count: 1 },
    { id: 'src_unknown', error_count: 1 },
  ] });
  assert.deepEqual(res, { ok: true, updated: 2, skipped: 1 });

  const [bca, chatime] = s.ss.getSheetByName('sources').objects();
  assert.equal(bca.last_fetched_at, '2026-11-02T06:01:00+07:00');
  assert.equal(bca.error_count, 3);
  assert.equal(bca.last_error, 'HTTP 503');
  assert.equal(bca.status, 'error');
  assert.equal(bca.target, 'https://promo.bca.co.id/id/all/c/fnb', 'setup column must not change');
  assert.equal(bca.notes, 'my note', 'setup column must not change');
  assert.equal(chatime.status, 'paused', 'a paused source stays paused');
  assert.equal(chatime.error_count, 1);
  assert.equal(s.ss.getSheetByName('sources').objects().length, 2, 'unknown ids are not added');
});

test('upsert_raw_items adds, updates, defaults status and fetched_at, and caps text', () => {
  const s = loadCode(tabs());
  let res = s.post('upsert_raw_items', { raw_items: [
    { id: 'src_ramenya_katalog:691830', source_id: 'src_ramenya_katalog', ref: '691830',
      url: 'https://aggregator.example/promo-es-krim-rp5000/', posted_at: '2026-10-29T12:53:53+07:00',
      text: 'x'.repeat(5000) },
  ] });
  assert.deepEqual(res, { ok: true, added: 1, updated: 0, pruned: 0 });
  let [item] = s.ss.getSheetByName('raw_items').objects();
  assert.equal(item.status, 'new');
  assert.equal(item.text.length, 5000, 'text up to 6,000 characters is kept whole');
  assert.match(item.fetched_at, /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\+07:00$/);

  res = s.post('upsert_raw_items', { raw_items: [
    { id: 'src_ramenya_katalog:691830', source_id: 'src_ramenya_katalog', status: 'parsed', attempts: 1,
      promo_ids: ['promo_a', 'promo_b'] },
  ] });
  assert.deepEqual(res, { ok: true, added: 0, updated: 1, pruned: 0 });
  [item] = s.ss.getSheetByName('raw_items').objects();
  assert.equal(item.status, 'parsed');
  assert.equal(item.promo_ids, 'promo_a,promo_b');
  assert.equal(item.url, 'https://aggregator.example/promo-es-krim-rp5000/', 'fields not sent are kept');
});

test('upsert_raw_items caps text at 6,000 characters', () => {
  const s = loadCode(tabs());
  s.post('upsert_raw_items', { raw_items: [{ id: 'src_a:1', source_id: 'src_a', ref: '1', text: 'y'.repeat(9000) }] });
  assert.equal(s.ss.getSheetByName('raw_items').objects()[0].text.length, 6000);
});

test('GET outlets returns open outlets only, with the fields used for matching', () => {
  const s = loadCode(tabs({ outlets: [['id', 'brand_id', 'name', 'address', 'mall_name', 'format', 'status', 'h3_r8'],
    ['out_osm_n1', 'brand_ramenya', 'RamenYa Margo', 'Jl. Margonda', 'Margo City', 'Ramen Ya', 'open', '88658c1a9bfffff'],
    ['out_osm_n2', 'brand_ramenya', 'RamenYa Old', '', '', 'Ramen Ya', 'closed', '88658c1a9bfffff'],
    ['out_manual_1', 'brand_hokben', 'HokBen Kokas', '', 'Kota Kasablanka', 'HokBen', '', ''],
  ] }));
  const res = s.get({ mode: 'outlets', token: 'w' });
  assert.equal(res.ok, true);
  assert.deepEqual(res.outlets.map((o) => o.id), ['out_osm_n1', 'out_manual_1'], 'closed outlets are left out; empty status means open');
  assert.deepEqual({ ...res.outlets[0] }, { id: 'out_osm_n1', brand_id: 'brand_ramenya', name: 'RamenYa Margo',
    address: 'Jl. Margonda', mall_name: 'Margo City', format: 'Ramen Ya' });
  assert.equal(s.get({ mode: 'outlets', token: 'r' }).error, 'unauthorized');
});

test('upsert_raw_items prunes rows fetched more than 90 days ago', () => {
  const s = loadCode(tabs({ raw_items: [RAW_HEADER,
    ['src_a:1', 'src_a', '1', '', '', 'old', 'parsed', 1, '', '', iso(91 * DAY)],
    ['src_a:2', 'src_a', '2', '', '', 'recent', 'failed', 2, 'timeout', '', iso(10 * DAY)],
  ] }));
  const res = s.post('upsert_raw_items', { raw_items: [{ id: 'src_a:3', source_id: 'src_a', ref: '3' }] });
  assert.deepEqual(res, { ok: true, added: 1, updated: 0, pruned: 1 });
  const ids = s.ss.getSheetByName('raw_items').objects().map((r) => r.id);
  assert.deepEqual(ids, ['src_a:2', 'src_a:3']);
});

test('upsert_raw_items rejects an item without id or source_id, or with a bad status', () => {
  const s = loadCode(tabs());
  assert.match(s.post('upsert_raw_items', { raw_items: [{ id: 'x' }] }).error, /needs id and source_id/);
  assert.match(s.post('upsert_raw_items', { raw_items: [{ id: 'x', source_id: 's', status: 'done' }] }).error, /bad status/);
  assert.match(s.post('upsert_raw_items', {}).error, /need raw_items/);
  assert.equal(s.ss.getSheetByName('raw_items').objects().length, 0);
});

test('GET raw_items filters by status and validates it', () => {
  const s = loadCode(tabs({ raw_items: [RAW_HEADER,
    ['src_a:1', 'src_a', '1', 'u1', '', 't1', 'failed', 2, 'timeout', '', iso(DAY)],
    ['src_a:2', 'src_a', '2', 'u2', '', 't2', 'parsed', 1, '', 'promo_x', iso(DAY)],
  ] }));
  const res = s.get({ mode: 'raw_items', status: 'failed', token: 'w' });
  assert.equal(res.ok, true);
  assert.deepEqual(res.raw_items.map((r) => r.id), ['src_a:1']);
  assert.equal(res.raw_items[0].attempts, 2);
  assert.match(s.get({ mode: 'raw_items', status: 'all', token: 'w' }).error, /status must be one of/);
});

test('GET raw_item_refs groups stored refs by source, any status', () => {
  const s = loadCode(tabs({ raw_items: [RAW_HEADER,
    ['src_a:1', 'src_a', '1', 'u1', '', 't1', 'failed', 2, 'timeout', '', iso(DAY)],
    ['src_a:2', 'src_a', '2', 'u2', '', 't2', 'parsed', 1, '', 'promo_x', iso(DAY)],
    ['src_b:691830', 'src_b', 691830, 'u3', '', 't3', 'new', 0, '', '', iso(DAY)],
  ] }));
  const res = s.get({ mode: 'raw_item_refs', token: 'w' });
  assert.equal(res.ok, true);
  assert.deepEqual({ ...res.refs }, { src_a: ['1', '2'], src_b: ['691830'] }, 'numeric refs come back as strings');
  assert.equal(s.get({ mode: 'raw_item_refs', token: 'r' }).error, 'unauthorized');
});

test('upsert_promos adds web promos with the new columns and clears the feed cache', () => {
  const s = loadCode(tabs());
  s.cache.set('mdm_data', '{}');
  const res = s.post('upsert_promos', { promos: [{
    id: 'promo_ramenya_eskrim5', brand_id: 'brand_ramenya', title: 'Es Krim Rp5.000', deal: 'Tiga rasa',
    channel: 'dine_in', valid_from: '2026-10-29', outlet_ids: ['out_osm_n1', 'out_osm_n2'], status: 'active',
    action_url: 'https://aggregator.example/promo-es-krim-rp5000/',
    source_ids: ['src_ramenya_katalog', 'src_ramenya_giladiskon'], source_refs: ['src_ramenya_katalog:691830'],
    dedupe_key: 'brand_ramenya|es krim rp5.000|2026-10-29|', confidence: 0.9,
  }] });
  assert.deepEqual(res, { ok: true, added: 1, updated: 0, skipped: 0 });
  assert.equal(s.cache.has('mdm_data'), false);

  const sheet = s.ss.getSheetByName('promos');
  const header = sheet.data[0];
  for (const col of ['source_ids', 'source_refs', 'dedupe_key', 'confidence', 'first_seen_at', 'last_seen_at']) {
    assert.ok(header.includes(col), 'missing column ' + col);
  }
  assert.deepEqual(header.slice(0, PROMO_HEADER.length), PROMO_HEADER, 'existing columns keep their names and order');

  const pr = sheet.objects().find((r) => r.id === 'promo_ramenya_eskrim5');
  assert.equal(pr.source, 'web');
  assert.equal(pr.outlet_ids, 'out_osm_n1,out_osm_n2');
  assert.equal(pr.source_ids, 'src_ramenya_katalog,src_ramenya_giladiskon');
  assert.match(pr.first_seen_at, /^\d{4}-\d{2}-\d{2}$/);
  assert.equal(pr.last_seen_at, pr.first_seen_at);
});

test('upsert_promos never rewrites a manual row, and updates web rows in place', () => {
  const s = loadCode(tabs({ promos: [PROMO_HEADER.concat(['first_seen_at', 'last_seen_at']),
    ['promo_hokben_bca28', 'brand_hokben', '', 'Mine', 'Cek syarat', 'dine_in', '', '', '', '', 'active', '', 'manual', '', ''],
    ['promo_ramenya_bundle40', 'brand_ramenya', '', 'Bundle Deals', 'old deal', 'delivery', '', '', '', '', 'hidden', '', 'web', '2026-10-20', '2026-10-20'],
  ] }));
  const sheet = s.ss.getSheetByName('promos');
  sheet.writes = [];

  const res = s.post('upsert_promos', { promos: [
    { id: 'promo_hokben_bca28', title: 'Fetcher title', status: 'active' },
    { id: 'promo_ramenya_bundle40', deal: 'new deal', status: 'active' },
  ] });
  assert.deepEqual(res, { ok: true, added: 0, updated: 1, skipped: 1 });

  const [manual, web] = sheet.objects();
  assert.equal(manual.title, 'Mine');
  assert.equal(manual.source, 'manual');
  assert.equal(web.deal, 'new deal');
  assert.equal(web.status, 'active', 'a hidden web promo comes back with the fetcher status');
  assert.equal(web.title, 'Bundle Deals', 'fields not sent are kept');
  assert.equal(web.first_seen_at, '2026-10-20', 'first_seen_at never moves');
  assert.ok(!sheet.writes.some((w) => w.row <= 2 && w.row + w.numRows - 1 >= 2), 'the manual row (sheet row 2) was written');
});

test('upsert_promos adds source_ids and source_refs to the row, and writes the M4 columns', () => {
  const s = loadCode(tabs({ promos: [PROMO_HEADER.concat(['source_ids', 'source_refs', 'first_seen_at', 'last_seen_at']),
    ['promo_a1b2', 'brand_ramenya', '', 'Diskon 50%', '50%', 'all', '', '', '', '', 'active', '', 'web',
      'src_ramenya_katalog', 'src_ramenya_katalog:1001', '2026-10-20', '2026-10-20'],
  ] }));
  s.post('upsert_promos', { promos: [{ id: 'promo_a1b2', status: 'active',
    source_ids: ['src_bca_fnb', 'src_ramenya_katalog'], source_refs: ['src_bca_fnb:9f3a'],
    recurrence: 'setiap Jumat', payment_methods: ['Kartu Kredit BCA', 'QRIS'] }] });
  const [pr] = s.ss.getSheetByName('promos').objects();
  assert.equal(pr.source_ids, 'src_ramenya_katalog,src_bca_fnb', 'a second source adds to the list, no duplicates');
  assert.equal(pr.source_refs, 'src_ramenya_katalog:1001,src_bca_fnb:9f3a');
  assert.equal(pr.recurrence, 'setiap Jumat');
  assert.equal(pr.payment_methods, 'Kartu Kredit BCA,QRIS');
  assert.equal(pr.status, 'active');
});

test('upsert_promos rejects ids that are not promo_ ids', () => {
  const s = loadCode(tabs());
  assert.match(s.post('upsert_promos', { promos: [{ id: 'brand_x' }] }).error, /promo id must look like/);
  assert.match(s.post('upsert_promos', { promos: [{ id: 'promo_Upper' }] }).error, /promo id must look like/);
});

test('setupPromoTabs creates tabs, adds promos columns, seeds once, and is safe to rerun', () => {
  const t = tabs();
  delete t.sources;
  delete t.raw_items;
  const s = loadCode(t);

  // Spread: objects from the vm context have its own Object prototype.
  assert.deepEqual({ ...s.ctx.setupPromoTabs() }, { seeded_sources: 9 });
  assert.deepEqual(s.ss.getSheetByName('raw_items').data[0], RAW_HEADER);
  const sources = s.ss.getSheetByName('sources').objects();
  assert.equal(sources.length, 9);
  assert.deepEqual(sources.filter((x) => x.status === 'paused').map((x) => x.id).sort(),
    ['src_chatime_giladiskon', 'src_ramenya_qpon', 'src_richeese_katalog']);
  assert.ok(sources.every((x) => x.type === 'web_page'), 'web_page is the only launch type');
  assert.ok(s.ss.getSheetByName('promos').data[0].includes('source_ids'));

  s.ss.getSheetByName('sources').put(2, SOURCE_HEADER.indexOf('notes') + 1, 'edited by hand');
  assert.deepEqual({ ...s.ctx.setupPromoTabs() }, { seeded_sources: 0 });
  assert.equal(s.ss.getSheetByName('sources').objects().length, 9);
  assert.equal(s.ss.getSheetByName('sources').objects()[0].notes, 'edited by hand');
});

test('existing calls still work: brands, feed, upsert_outlets', () => {
  const s = loadCode(tabs());
  assert.equal(s.get({ mode: 'brands', token: 'w' }).brands[0].id, 'brand_ramenya');
  assert.equal(s.get({ mode: 'swipe', token: 'r' }).ok, true);
  assert.match(s.post('upsert_outlets', { outlets: [] }).error, /need source and outlets/);
  const res = s.post('upsert_outlets', { source: 'osm', outlets: [{ id: 'out_osm_n1', brand_id: 'brand_ramenya', name: 'RamenYa Margo' }] });
  assert.deepEqual(res, { ok: true, added: 0, updated: 1, closed: 0 });
});
