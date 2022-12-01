import http from "k6/http";
import { check } from "k6";

export const options = {
  vus: 2,
  duration: '10s',
  thresholds: {
    'http_reqs{expected_response:true}': ['rate>10'],
  },
};

export default function () {
  check(http.get("http://localhost:8123/"), {
    "status is 200": (r) => r.status == 200,
  });
}
