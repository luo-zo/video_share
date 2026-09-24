import http from "k6/http";
import { check, sleep } from "k6";

const baseURL = (__ENV.BASE_URL || "http://host.docker.internal:18081").replace(/\/$/, "");
const profile = __ENV.PROFILE || "load";
const windowName = __ENV.WINDOW || "day";
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
        ranking: { executor: "constant-vus", vus, duration },
      },
      thresholds: { checks: ["rate==1"], http_req_failed: ["rate==0"] },
    };

export default function () {
  const response = http.get(
    `${baseURL}/api/v1/videos/ranking?window=${encodeURIComponent(windowName)}&page=1&page_size=20`,
    { tags: { endpoint: "ranking", window: windowName, cache_mode: __ENV.CACHE_MODE || "unspecified" } },
  );
  check(response, {
    "ranking returns HTTP 200": (r) => r.status === 200,
    "ranking envelope and current visibility are valid": (r) => {
      if (r.status !== 200) return false;
      try {
        const data = r.json("data");
        return data.window === windowName
          && Array.isArray(data.items)
          && data.items.every((item, index) => item.status === "ready"
            && item.visibility === "public"
            && item.id > 0
            && item.rank === index + 1);
      } catch (_) {
        return false;
      }
    },
  });
  sleep(0.1);
}
