import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  scenarios: {
    list_stress: {
      executor: 'constant-vus',
      vus: 100,
      duration: '2m',
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<2000'],
    http_req_failed: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.API_URL || 'http://localhost:8080';

export default function () {
  const orgID = 'org-load-test';
  const projectID = 'proj-load-test';
  const baseURL = `${BASE_URL}/api/v1/orgs/${orgID}/projects/${projectID}/literature-reviews`;

  const pageSizes = [10, 25, 50, 100];
  const pageSize = pageSizes[Math.floor(Math.random() * pageSizes.length)];

  const res = http.get(`${baseURL}?page_size=${pageSize}`);
  check(res, {
    'status is 200': (r) => r.status === 200,
    'response is JSON': (r) => r.headers['Content-Type'].includes('application/json'),
    'has reviews array': (r) => JSON.parse(r.body).reviews !== undefined,
    'has total_count': (r) => JSON.parse(r.body).total_count !== undefined,
  });

  sleep(0.5);
}
