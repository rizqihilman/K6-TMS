Perintah k6

▶️ Menjalankan script
k6 run test.js
//Menjalankan file test.js dengan opsi default (output di terminal).

▶️ Menjalankan dengan output ke dashboard (grafik)
k6 run --out dashboard test.js
//Secara default dashboard akan berjalan di http://127.0.0.1:5665.

Pastikan tidak ada proses lain yang memakai port tersebut.

▶️ Menjalankan dashboard dengan port kustom
//Jika port 5665 bentrok, ganti dengan port lain (misalnya 8082):

k6 run --out dashboard=http.addr=0.0.0.0:8082 test.js
//Sekarang bukanya di: http://127.0.0.1:8082

▶️ Menampilkan daftar opsi
k6 run --help
atau
k6 --help

------------------------------------------------------------------------------------------------------------------------------------------
✅ Langkah Push Perubahan ke GitHub (Versi Sederhana)
1. Cek status
Buka terminal (di VS Code tekan Ctrl + `` ) lalu jalankan:

git status
Ini untuk lihat file mana saja yang berubah.

2. Tambahkan perubahan ke staging
Tambahkan semua file yang berubah:

git add .
Titik (.) artinya semua file. Atau kamu bisa spesifik: git add nama_file.

3. Commit perubahan
Beri catatan perubahan kamu:
git commit -m "Deskripsi singkat perubahan"
Contoh:
git commit -m "Update halaman login dan perbaikan bug validasi"

4. Push ke GitHub
git push origin nama-branch
Biasanya branch default-nya adalah main atau master, jadi:

git push origin main

🔍 Cara Cek Nama Branch Saat Ini
Kalau kamu tidak yakin branch-nya apa:
git branch
Branch yang aktif akan ada tanda *.

Kalau kamu bekerja dalam tim, pastikan untuk pull dulu sebelum push:
git pull origin main
Kalau kamu mau aku buatkan skrip singkat atau alias supaya langkah-langkah itu bisa disingkat (misal jadi 1 perintah), bisa juga! Mau?

 1. Jalankan Langsung di Terminal (Command Sementara)

git add . && git commit -m "update project" && git push origin main

Kamu bisa ganti "update project" dengan pesan commit yang kamu inginkan, misalnya:

git add . && git commit -m "perbaikan validasi form login" && git push origin main