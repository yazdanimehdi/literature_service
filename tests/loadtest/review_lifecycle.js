import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  scenarios: {
    concurrent_reviews: {
      executor: 'constant-vus',
      vus: 50,
      duration: '5m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<5000'],
    http_req_failed: ['rate<0.05'],
    'checks': ['rate>0.95'],
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

export default function () {
  const orgID = 'org-load-test';
  const projectID = 'proj-load-test';
  const baseURL = `${BASE_URL}/api/v1/orgs/${orgID}/projects/${projectID}/literature-reviews`;

  const startPayload = JSON.stringify({
    query: `load test query ${Date.now()} VU${__VU}`,
    max_expansion_depth: 0,
  });

  const startRes = http.post(baseURL, startPayload, {
    headers: { 'Content-Type': 'application/json' },
  });

  const startOk = check(startRes, {
    'start: status 201': (r) => r.status === 201,
    'start: has review_id': (r) => JSON.parse(r.body).review_id !== undefined,
  });

  if (!startOk) {
    sleep(1);
    return;
  }

  const reviewID = JSON.parse(startRes.body).review_id;

  for (let i = 0; i < 10; i++) {
    sleep(3);
    const statusRes = http.get(`${baseURL}/${reviewID}`);
    check(statusRes, {
      'status: 200': (r) => r.status === 200,
    });
    const status = JSON.parse(statusRes.body).status;
    if (['completed', 'partial', 'failed', 'cancelled'].includes(status)) {
      break;
    }
  }

  const listRes = http.get(`${baseURL}?page_size=10`);
  check(listRes, {
    'list: status 200': (r) => r.status === 200,
  });

  sleep(1);
}
