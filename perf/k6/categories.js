import http from "k6/http";
import { check, sleep } from "k6";

const baseURL = (__ENV.BASE_URL || "http://host.docker.internal:18081").replace(/\/$/, "");
const profile = __ENV.PROFILE || "load";
const vus = Number(__ENV.VUS || 5);
const duration = __ENV.DURATION || "15s";

export const options = profile === "cold"
  ? {
      vus: 1,
      iterations: 1,
      summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
      thresholds: { checks: ["rate==1"], http_req_failed: ["rate==0"] },
    }
  : {
      summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
      scenarios: {
        categories: { executor: "constant-vus", vus, duration },
      },
      thresholds: { checks: ["rate==1"], http_req_failed: ["rate==0"] },
    };

export default function () {
  const response = http.get(`${baseURL}/api/v1/categories`, {
    tags: { endpoint: "categories", cache_mode: __ENV.CACHE_MODE || "unspecified" },
  });
  check(response, {
    "categories returns HTTP 200": (r) => r.status === 200,
    "categories envelope contains enabled items": (r) => {
      if (r.status !== 200) return false;
      try {
        const items = r.json("data.items");
        return Array.isArray(items) && items.every((item) => item.enabled === true && item.id > 0);
      } catch (_) {
        return false;
      }
    },
  });
  sleep(0.1);
}
