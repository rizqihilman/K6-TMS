import http from 'k6/http';
import { sleep } from 'k6';
import { handleSummary } from './handleSummary.js';

export const options = {
  stages: [
    { duration: '1m', target: 50 }, // ramp-up

  ],
};
export default function () {
  // 1. Login
  const loginRes = http.post('https://tms.ttnt.arkamaya.net/login/', {
    username: 'admin-kasir@ttnt.co.id',
    password: '123123',
  });

  // 2. Ambil cookie dari response login
  const cookies = loginRes.cookies['ci_sessions'];
  let cookieHeader = 's67pgo29kq2ue8o2rg90onsf8lsa06ud';
  if (cookies && cookies.length > 0) {
    cookieHeader = `ci_sessions=${cookies[0].value}`;
  }

  // 3. Kirim request dengan cookie
  const headers = {
    Cookie: cookieHeader,
  };
  const res = http.get('https://tms.ttnt.arkamaya.net/grouping_route', { headers });

// Cek status code di dalam fungsi
  if (res.status === 200) {
    console.log('Berhasil akses grouping_route');
  } else {
    console.log(`Gagal akses grouping_route. Status: ${res.status}`);
    console.log(res.body);
  }

  sleep(1);
}

export { handleSummary };
// ...existing code...

