import http from "k6/http";
import { check } from "k6";

const BASE = __ENV.BASE_URL || "http://localhost:8080";
const API_KEY = __ENV.API_KEY || "";
const RATE = Number(__ENV.RATE || 200);
const DURATION = __ENV.DURATION || "60s";

const writeHeaders = { "Content-Type": "application/json", "X-API-Key": API_KEY };

export const options = {
  scenarios: {
    read: {
      executor: "constant-arrival-rate",
      exec: "read",
      rate: Math.round(RATE * 0.9),
      timeUnit: "1s",
      duration: DURATION,
      preAllocatedVUs: 50,
      maxVUs: 200,
    },
    write: {
      executor: "constant-arrival-rate",
      exec: "write",
      rate: Math.max(1, Math.round(RATE * 0.1)),
      timeUnit: "1s",
      duration: DURATION,
      preAllocatedVUs: 10,
      maxVUs: 50,
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    "http_req_duration{scenario:read}": ["p(95)<300"],
    "http_req_duration{scenario:write}": ["p(95)<500"],
  },
};

export function setup() {
  const ids = [];
  for (let i = 0; i < 200; i++) {
    const res = http.post(`${BASE}/api/v1/products`, JSON.stringify({ name: `seed-${i}`, price: `${i}.99` }), { headers: writeHeaders });
    check(res, { "seed created": (r) => r.status === 201 });
    ids.push(res.json("id"));
  }
  return { ids };
}

export function read(data) {
  if (Math.random() < 0.5) {
    const res = http.get(`${BASE}/api/v1/products?limit=20`, { tags: { name: "list" } });
    check(res, { "list 200": (r) => r.status === 200 });
    return;
  }

  const id = data.ids[Math.floor(Math.random() * data.ids.length)];
  const res = http.get(`${BASE}/api/v1/products/${id}`, { tags: { name: "get" } });
  check(res, { "get 200": (r) => r.status === 200 });
}

export function write() {
  const res = http.post(`${BASE}/api/v1/products`, JSON.stringify({ name: "load", price: "10.00" }), {
    headers: writeHeaders,
    tags: { name: "create" },
  });
  check(res, { "create 201": (r) => r.status === 201 });
}
