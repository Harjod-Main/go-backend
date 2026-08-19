import http from 'k6/http';
import { Counter } from 'k6/metrics';
import { check, sleep } from 'k6';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.1.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8082';
const upstreamHits = new Counter('upstream_hits');

export const options = {
  stages: [
    { duration: '15s', target: 20 },
    { duration: '30s', target: 20 },
    { duration: '15s', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500'],
  },
};

export default function () {
  const res = http.get(`${BASE_URL}/liveness`);
  const upstream = res.headers['X-Upstream'] || 'unknown';
  upstreamHits.add(1, { upstream });
  check(res, {
    'status is 200': (r) => r.status === 200,
    'has X-Upstream': () => upstream !== 'unknown',
    [`backend ${upstream}`]: () => true,
  });
  sleep(0.1);
}

function backendCounts(data) {
  const counts = {};
  const add = (name, n) => {
    if (!name || !n) return;
    counts[name] = (counts[name] || 0) + n;
  };

  for (const [key, metric] of Object.entries(data.metrics || {})) {
    const tagged = key.match(/^upstream_hits\{.*upstream:([^,}]+)/);
    if (tagged) add(tagged[1].trim(), metric.values && metric.values.count);
  }

  const walk = (group) => {
    if (!group) return;
    const checks = group.checks;
    const list = Array.isArray(checks) ? checks : Object.values(checks || {});
    for (const c of list) {
      const name = c.name || '';
      if (name.startsWith('backend ')) add(name.slice(8), c.passes || 0);
    }
    const groups = group.groups;
    const nested = Array.isArray(groups) ? groups : Object.values(groups || {});
    nested.forEach(walk);
  };
  walk(data.root_group);
  return counts;
}

export function handleSummary(data) {
  const counts = backendCounts(data);
  const entries = Object.entries(counts).sort((a, b) => a[0].localeCompare(b[0]));
  const total = entries.reduce((s, [, n]) => s + n, 0);
  let split = '\n  backend split (X-Upstream):\n';
  if (!total) {
    split += '    (none — is the LB up on :8082?)\n';
  } else {
    for (const [name, n] of entries) {
      split += `    ${name}  ${n}  (${((n / total) * 100).toFixed(1)}%)\n`;
    }
  }
  return {
    stdout: textSummary(data, { indent: ' ', enableColors: true }) + split,
  };
}
