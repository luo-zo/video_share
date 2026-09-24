import http from "k6/http";
import { check, sleep } from "k6";

const baseURL = (__ENV.BASE_URL || "http://host.docker.internal:18081").replace(/\/$/, "");
const segmentPath = __ENV.SEGMENT_PATH || "";
const vus = Number(__ENV.VUS || 5);
const duration = __ENV.DURATION || "15s";

if (
  !/^\/api\/v1\/videos\/\d+\/hls\/[A-Za-z0-9._/-]+\.ts$/.test(segmentPath) ||
  segmentPath.includes("..")
) {
  throw new Error("SEGMENT_PATH must be a validated HLS segment path from the isolated public video");
}

export const options = {
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  scenarios: {
    hlsSegment: { executor: "constant-vus", vus, duration },
  },
  thresholds: { checks: ["rate==1"], http_req_failed: ["rate==0"] },
};

export default function () {
  const response = http.get(`${baseURL}${segmentPath}`, {
    tags: { endpoint: "hls-segment", cache_mode: "object-read" },
  });
  check(response, {
    "HLS segment returns HTTP 200": (r) => r.status === 200,
    "HLS response is MPEG-TS": (r) => (r.headers["Content-Type"] || "").includes("video/mp2t"),
    "HLS segment body is non-empty": (r) => typeof r.body === "string" && r.body.length > 1024,
  });
  sleep(0.1);
}
