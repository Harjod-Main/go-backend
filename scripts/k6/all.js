import http from 'k6/http';
import { Counter } from 'k6/metrics';
import { check, sleep } from 'k6';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.1.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8082';
const TOKEN = __ENV.TOKEN || '';
const INCLUDE_GOOGLE = __ENV.INCLUDE_GOOGLE === '1';
const status429 = new Counter('status_429');
const byStatus = new Counter('http_by_status');

export const options = {
  vus: Number(__ENV.VUS || 1),
  iterations: Number(__ENV.ITERATIONS || 1),
};

function authHeaders() {
  return TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {};
}

function hit(name, res) {
  byStatus.add(1, { endpoint: name, status: String(res.status) });
  if (res.status === 429) status429.add(1, { endpoint: name });
  check(res, { [`${name} ok`]: (r) => r.status === 200 });
  return res;
}

function jsonData(res) {
  try {
    return (res.json() || {}).data;
  } catch (_) {
    return null;
  }
}

function discoverPrivileges(placeId) {
  const res = http.get(`${BASE_URL}/api/v1/places/${placeId}/privileges`);
  const data = jsonData(res) || {};
  const out = { stampId: '', reserveId: '', evId: '' };
  const stamps = data.validation_parking || [];
  if (stamps[0] && stamps[0].validation) out.stampId = stamps[0].validation.validation_id || '';
  for (const area of data.parking_area || []) {
    if (!out.reserveId && area.reserved && area.reserved[0]) out.reserveId = area.reserved[0].reserved_id || '';
    if (!out.evId && area.ev_charger && area.ev_charger[0]) out.evId = area.ev_charger[0].ev_charger_id || '';
  }
  return out;
}

export function setup() {
  const list = http.get(`${BASE_URL}/api/v1/places`);
  if (list.status !== 200) {
    throw new Error(`setup GET /api/v1/places failed: ${list.status} ${list.body}`);
  }
  const places = jsonData(list) || [];
  if (!places.length) throw new Error('setup: no places');

  const ids = {
    placeId: places[0].place_id,
    placeIds: places.slice(0, 3).map((p) => p.place_id),
    stampId: '',
    reserveId: '',
    evId: '',
    googlePlaceId: '',
  };

  for (const p of places.slice(0, 20)) {
    const found = discoverPrivileges(p.place_id);
    ids.stampId = ids.stampId || found.stampId;
    ids.reserveId = ids.reserveId || found.reserveId;
    ids.evId = ids.evId || found.evId;
    if (ids.stampId && ids.reserveId && ids.evId) break;
  }

  if (INCLUDE_GOOGLE) {
    const ac = http.get(`${BASE_URL}/api/v1/places/autocomplete?q=BTS&lat=13.7563&lng=100.5018`);
    const predictions = ((jsonData(ac) || {}).predictions) || [];
    if (predictions[0] && predictions[0].placeId) ids.googlePlaceId = predictions[0].placeId;
  }

  return ids;
}

export default function (ids) {
  const id = ids.placeId;
  const headers = authHeaders();

  hit('liveness', http.get(`${BASE_URL}/liveness`));
  hit('readiness', http.get(`${BASE_URL}/readiness`));

  const debug = http.get(`${BASE_URL}/debug/client-ip`);
  check(debug, { 'debug/client-ip 200 or 404': (r) => r.status === 200 || r.status === 404 });

  hit('places', http.get(`${BASE_URL}/api/v1/places`));
  hit('autocomplete empty', http.get(`${BASE_URL}/api/v1/places/autocomplete`));
  hit('card', http.get(`${BASE_URL}/api/v1/places/${id}/card`));
  hit('rate', http.get(`${BASE_URL}/api/v1/places/${id}/rate`));
  hit('privileges', http.get(`${BASE_URL}/api/v1/places/${id}/privileges`));
  hit('quote', http.get(`${BASE_URL}/api/v1/places/${id}/quote?hours=3`));
  hit('reaction', http.get(`${BASE_URL}/api/v1/places/${id}/reaction`, { headers }));
  hit('reviews', http.get(`${BASE_URL}/api/v1/places/${id}/reviews`));
  hit('leaderboard', http.get(`${BASE_URL}/api/v1/leaderboard?limit=10`));
  hit('quotes batch', http.post(
    `${BASE_URL}/api/v1/quotes`,
    JSON.stringify({ hours: 3, placeIds: ids.placeIds }),
    { headers: { 'Content-Type': 'application/json' } },
  ));

  if (ids.stampId) hit('privilege stamp', http.get(`${BASE_URL}/api/v1/privileges/stamp/${ids.stampId}`));
  if (ids.reserveId) hit('privilege reserve', http.get(`${BASE_URL}/api/v1/privileges/reserve/${ids.reserveId}`));
  if (ids.evId) hit('privilege ev', http.get(`${BASE_URL}/api/v1/privileges/ev/${ids.evId}`));

  if (INCLUDE_GOOGLE && ids.googlePlaceId) {
    hit('google details', http.get(`${BASE_URL}/api/v1/places/details/${ids.googlePlaceId}`));
  }

  if (TOKEN) {
    hit('auth me', http.get(`${BASE_URL}/api/v1/auth/me`, { headers }));
    hit('profile', http.get(`${BASE_URL}/api/v1/profile`, { headers }));
    hit('credit-points', http.get(`${BASE_URL}/api/v1/me/credit-points`, { headers }));
    hit('stats', http.get(`${BASE_URL}/api/v1/me/stats`, { headers }));
    hit('check-ins', http.get(`${BASE_URL}/api/v1/me/check-ins`, { headers }));
    hit('reports', http.get(`${BASE_URL}/api/v1/me/reports`, { headers }));
    hit('notif prefs', http.get(`${BASE_URL}/api/v1/me/notification-preferences`, { headers }));
  }

  sleep(Number(__ENV.SLEEP || 0));
}

export function handleSummary(data) {
  const limited = (data.metrics.status_429 && data.metrics.status_429.values.count) || 0;
  return {
    stdout:
      textSummary(data, { indent: ' ', enableColors: true }) +
      `\n  http 429: ${limited}\n  writes skipped (would mutate the real DB)\n  google details skipped unless INCLUDE_GOOGLE=1\n  auth GETs skipped unless TOKEN is set\n`,
  };
}
