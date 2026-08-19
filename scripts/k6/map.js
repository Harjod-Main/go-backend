import http from 'k6/http';
import { Counter } from 'k6/metrics';
import { check, sleep } from 'k6';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.1.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8082';
const status429 = new Counter('status_429');
const upstreamHits = new Counter('upstream_hits');

export const options = {
  vus: Number(__ENV.VUS || 1),
  duration: __ENV.DURATION || '45s',
  thresholds: {
    'http_req_duration{expected_response:true}': ['p(95)<2000'],
  },
};

export function setup() {
  const res = http.get(`${BASE_URL}/api/v1/places`);
  if (res.status !== 200) {
    throw new Error(`setup GET /api/v1/places failed: ${res.status} ${res.body}`);
  }
  const places = (res.json() || {}).data || [];
  if (!places.length || !places[0].place_id) {
    throw new Error('setup: no places in map list');
  }
  return { placeId: places[0].place_id };
}

export default function (data) {
  const id = data.placeId;
  const reqs = [
    ['list', `${BASE_URL}/api/v1/places`],
    ['card', `${BASE_URL}/api/v1/places/${id}/card`],
    ['rate', `${BASE_URL}/api/v1/places/${id}/rate`],
    ['quote', `${BASE_URL}/api/v1/places/${id}/quote?hours=3`],
  ];

  for (const [name, url] of reqs) {
    const res = http.get(url, { tags: { endpoint: name } });
    const upstream = res.headers['X-Upstream'] || 'unknown';
    upstreamHits.add(1, { upstream });
    if (res.status === 429) status429.add(1, { endpoint: name });
    check(res, {
      [`${name} 200`]: (r) => r.status === 200,
      [`backend ${upstream}`]: () => true,
    });
  }

  sleep(Number(__ENV.SLEEP || 2));
}

function backendCounts(data) {
  const counts = {};
  const add = (name, n) => {
    if (!name || !n) return;
    counts[name] = (counts[name] || 0) + n;
  };
  const walk = (group) => {
    if (!group) return;
    const list = Array.isArray(group.checks) ? group.checks : Object.values(group.checks || {});
    for (const c of list) {
      const name = c.name || '';
      if (name.startsWith('backend ')) add(name.slice(8), c.passes || 0);
    }
    const nested = Array.isArray(group.groups) ? group.groups : Object.values(group.groups || {});
    nested.forEach(walk);
  };
  walk(data.root_group);
  return counts;
}

export function handleSummary(data) {
  const counts = backendCounts(data);
  const entries = Object.entries(counts).sort((a, b) => a[0].localeCompare(b[0]));
  const total = entries.reduce((s, [, n]) => s + n, 0);
  let extra = '\n  backend split (X-Upstream):\n';
  if (!total) extra += '    (none)\n';
  else {
    for (const [name, n] of entries) {
      extra += `    ${name}  ${n}  (${((n / total) * 100).toFixed(1)}%)\n`;
    }
  }
  const limited = (data.metrics.status_429 && data.metrics.status_429.values.count) || 0;
  extra += `  http 429: ${limited}  (public map APIs cap at 60/min/IP per replica)\n`;
  return { stdout: textSummary(data, { indent: ' ', enableColors: true }) + extra };
}
