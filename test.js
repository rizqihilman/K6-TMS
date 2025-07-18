// ...existing code...
import http from 'k6/http';
import { sleep } from 'k6';
import { handleSummary } from './handleSummary.js';

export const options = {
  vus: 5,
  duration: '10s',
  // Hapus summary dari options
};

export default function () {
  http.get('https://test.k6.io/');
  sleep(1);
}

// Ekspor handleSummary di luar options
export { handleSummary };
// ...existing code...