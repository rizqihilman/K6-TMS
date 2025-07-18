import { htmlReport } from "https://raw.githubusercontent.com/benc-uk/k6-reporter/main/dist/bundle.js";

export function handleSummary(data) {
let report = htmlReport(data);
// Insert logo with improved alt text for accessibility
report = report.replace(
'<body>',
    `<body>
      <div style="display:flex; align-items:center; margin-bottom:20px;">
        <img src="./logo/ttnt.png" alt="Logo" style="width:100px; margin-right:10px;">
        <span style="font-size:1.5em; font-weight:bold;">PT. Toyota Tsusho Nusa Transport</span>
      </div>
  `
  );
//<p>K6 Load Test: ${dateString}</p>
  
return {
    "summary.html": report,
    "summary.json": JSON.stringify(data),
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
};
}
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';
// ...existing code...

const now = new Date();
const dateString = now.toLocaleString('id-ID', { timeZone: 'Asia/Jakarta' });
