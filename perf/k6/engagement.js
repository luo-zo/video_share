import http from "k6/http";
import { check, sleep } from "k6";

// A 401 from the out-of-band anonymous write probe is expected; the timed
// workload itself still requires every comment-list request to return 200.
http.setResponseCallback(http.expectedStatuses(200, 401));

const baseURL = (__ENV.BASE_URL || "http://host.docker.internal:18081").replace(/\/$/, "");
const videoID = Number(__ENV.VIDEO_ID || 0);
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
        comments: { executor: "constant-vus", vus, duration },
      },
      thresholds: { checks: ["rate==1"], http_req_failed: ["rate==0"] },
    };

export function setup() {
  if (!Number.isSafeInteger(videoID) || videoID <= 0) {
    throw new Error("VIDEO_ID must identify a public video in the isolated test database");
  }
  // This is a permission probe, not part of the timed VU workload. The API
  // must reject an anonymous write before parsing or persisting a comment.
  const response = http.post(
    `${baseURL}/api/v1/videos/${videoID}/comments`,
    JSON.stringify({ content: "synthetic unauthenticated benchmark probe", request_id: "k6-anonymous-probe" }),
    { headers: { "Content-Type": "application/json" }, tags: { endpoint: "permission_probe" } },
  );
  const denied = check(response, {
    "anonymous comment creation is rejected": (r) => r.status === 401,
  });
  if (!denied) throw new Error(`anonymous comment write returned ${response.status}, expected 401`);
  return { videoID };
}

export default function (data) {
  const response = http.get(
    `${baseURL}/api/v1/videos/${data.videoID}/comments?page=1&page_size=20`,
    { tags: { endpoint: "comments", cache_mode: "mysql-read" } },
  );
  check(response, {
    "comments returns HTTP 200": (r) => r.status === 200,
    "comments envelope is structurally valid": (r) => {
      if (r.status !== 200) return false;
      try {
        const items = r.json("data.items");
        return Array.isArray(items) && items.every((item) => item.id > 0 && item.video_id === data.videoID);
      } catch (_) {
        return false;
      }
    },
  });
  sleep(0.1);
}
