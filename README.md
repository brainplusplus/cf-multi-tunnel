# CF Multi Tunnel Panel

Panel self-hosted untuk mengelola banyak Cloudflare Tunnel dari banyak akun Cloudflare,
dengan pemisahan jelas antara tunnel yang berjalan **di server ini** dan tunnel yang
berjalan **di perangkat lain**.

- Backend: Go + Echo + GORM + SQLite
- Frontend: React 19 + Vite + Tailwind 4 (dilayani oleh reverse proxy Go yang sama)
- Deploy: satu `docker compose up -d --build`

## Kebutuhan

- Docker + Docker Compose (panel menjalankan satu container `cloudflared` per tunnel lokal,
  jadi butuh akses ke `docker.sock`)
- Akun Cloudflare + API token (lihat [Permission token](#permission-token-cloudflare))
- Domain yang zone-nya ada di akun tersebut

## Quick start

```bash
cp .env.example .env
# ADMIN_PASSWORD default admin123 — tidak wajib diganti supaya simpel.
# Kalau nilainya masih lemah, backend hanya menulis peringatan di log.
docker compose up -d --build
```

Buka `http://<host>:7777` (ubah dengan `PANEL_PORT` di `.env`).

Kalau `PUBLIC_URL` bukan `http://localhost:7777`, ubah juga — nilai itu dipakai untuk
menampilkan alamat panel dan untuk validasi saat panel dipublikasikan lewat tunnel.

## Login pertama

- Email: `ADMIN_EMAIL` dari `.env`
- Password: `ADMIN_PASSWORD` dari `.env`

`ADMIN_PASSWORD` hanya dibaca saat tabel user masih kosong. Setelah itu ganti lewat
menu **Account → Change password**; mengubah `.env` tidak mengubah password yang sudah ada.

Tidak ada pendaftaran publik. User biasa dibuat oleh admin di halaman **Users**
(role `user` hanya melihat akun dan tunnel miliknya sendiri).

## Konsep

**Cloudflare account** — satu baris berisi kredensial untuk satu akun Cloudflare.
Dua cara menambah:

| Cara | Yang bisa dilakukan |
|---|---|
| API token | Semua: list tunnel, baca/tulis ingress, kelola DNS, start/stop |
| Import tunnel token | Hanya menyimpan kredensial satu tunnel; tidak bisa menulis ke Cloudflare |

Menambahkan API token untuk akun yang sebelumnya diimpor lewat tunnel token akan
**meng-upgrade baris yang sama** (bukan duplikat) dan langsung mengimpor tunnel serta
hostname yang sudah ada di Cloudflare.

**Tunnel** — dua field terpisah, keduanya penting:

- `location`:
  - `local` — cloudflared berjalan di server panel ini
  - `remote` — cloudflared berjalan di perangkat lain (laptop, NAS, server lain)
- `connector_mode`:
  - `docker` — panel yang menjalankan/mengawasi container cloudflared
  - `external` — panel tidak pernah menyalakan apa pun; Anda jalankan cloudflared sendiri

Panel hanya menjalankan Docker untuk `local` + `docker`. Tunnel `remote` dipantau
lewat Cloudflare API saja, jadi panel tidak pernah mengklaim mengawasi container di
perangkat lain. Kombinasi `remote/external` adalah default untuk tunnel hasil impor.

**Published app** — hostname (mis. `app.example.com`) yang diarahkan ke sebuah tunnel.
Panel menyusun ulang seluruh daftar ingress Cloudflare dari daftar app ini, selalu
diakhiri `http_status:404`, di bawah lock per-tunnel. DNS dikelola hanya jika
`manage_dns` aktif; saat impor, DNS diadopsi hanya kalau sudah ada CNAME ke
`<tunnel-id>.cfargotunnel.com`.

**Panel exposure** — admin bisa mempublikasikan dashboard ini sendiri lewat tunnel
(**Settings → Panel exposure**). Untuk tunnel `local`, alamat internal panel dipakai
otomatis. Untuk tunnel `remote`, Anda wajib mengisi alamat yang bisa dijangkau dari
perangkat itu (mis. `http://192.168.1.10:7777`); panel menolak alamat yang akan
memutar balik ke hostname publik atau `PUBLIC_URL`.

## Permission token Cloudflare

Buat token di Cloudflare Dashboard → My Profile → API Tokens → Create Token:

| Scope | Permission |
|---|---|
| Account | Cloudflare Tunnel · Edit |
| Account | Account Settings · Read |
| Zone | Zone · Read |
| Zone | DNS · Edit |

Token disimpan terenkripsi AES-256-GCM dan tidak pernah dikembalikan lewat API
(hanya `token_hint`). Kunci enkripsi ada di volume data; kalau kunci hilang, semua
token harus dimasukkan ulang.

## Data dan backup

Semua state ada di named volume `cfmt-data` (`/app/data`):

| File | Isi |
|---|---|
| `app.db` (+ `-wal`, `-shm`) | user, akun, tunnel, app, audit log, settings |
| `.jwt_secret` | kunci tanda tangan JWT (auto-generate) |
| `encryption.key` | kunci AES untuk token Cloudflare (auto-generate) |

`docker compose up -d --build`, `docker compose down`, dan rebuild image **tidak**
menghapus volume ini. Hanya `docker compose down -v` yang menghapusnya.

Backup panas (aman saat panel berjalan):

```powershell
.\scripts\backup.ps1        # Windows
./scripts/backup.sh         # Linux/macOS
```

Hasilnya di `backups/cfmt-<timestamp>.db`. Untuk restore lengkap, salin `app.db`,
`.jwt_secret`, dan `encryption.key` kembali ke volume saat container berhenti.
Backup DB saja tidak cukup: tanpa `encryption.key`, token Cloudflare di dalamnya
tidak bisa didekripsi.

## Upgrade

```bash
git pull
docker compose up -d --build
```

Data tetap. Kalau ada perubahan `.env.example`, bandingkan dengan `.env` Anda —
jangan menimpa `.env` yang sudah berisi kredensial.

## Keamanan

- **Tidak ada sign-up publik.** Akun dibuat admin. Ini disengaja: panel bisa
  dijangkau dari internet saat dipublikasikan lewat tunnel.
- **Login dibatasi**: 5 kegagalan dalam 15 menit mengunci email tersebut selama
  10 menit (berlaku juga untuk email yang tidak terdaftar). Counter ada di memori,
  jadi restart backend menghapus lockout.
- **Refresh token dirotasi** setiap dipakai; token lama yang dipakai ulang mencabut
  seluruh keluarga token tersebut.
- **Password admin pertama**: default `admin123` supaya start pertama simpel. Kalau
  nilainya masih `admin123` atau di bawah 10 karakter, backend menulis peringatan di
  log. Ganti lewat UI (**Account → Change password**, minimal 8 karakter) kalau panel
  bisa diakses dari internet. `ADMIN_PASSWORD` hanya dipakai saat tabel user kosong.
- Container backend berjalan sebagai **root** karena harus mengakses `docker.sock`
  untuk menjalankan container cloudflared. Ini setara dengan akses root ke host:
  batasi akses ke panel, dan jangan bagikan akun admin.
- Panel tidak menyimpan password Cloudflare, hanya API token terenkripsi.

## Troubleshooting

**Lupa password admin**
`ADMIN_PASSWORD` hanya dibaca saat tabel user masih kosong; setelah itu mengubah `.env`
tidak berpengaruh. Ganti password lewat **Account → Change password** selama masih bisa
login. Kalau benar-benar terkunci, pulihkan dari backup (`backups/*.db`) atau jalankan
ulang dengan volume kosong (data akun/tunnel ikut hilang).

**Tunnel lokal tidak connect**
- `docker compose logs backend` — cek error dari Docker/cloudflared
- Pastikan `/var/run/docker.sock` ter-mount dan image `cloudflared` bisa di-pull
- Cek log tunnel di UI (**Tunnels → detail → Logs**)

**Ingress tidak berubah di Cloudflare**
Hanya tunnel dengan `config_src=cloudflare` yang bisa diubah lewat API. Kalau
`config_src` adalah `local`, konfigurasi dikelola dari file `config.yml` di perangkat
tersebut; panel hanya bisa menampilkan file contohnya.

**Hostname sudah dipakai**
Satu hostname hanya boleh dimiliki satu app. Saat sync, hostname milik tunnel lain
dilewati dan dilaporkan, tidak direbut.

**Data hilang setelah deploy**
Hampir pasti volume terhapus (`down -v`) atau `DB_PATH`/`DATA_DIR` diarahkan ke luar
`/app/data`. Cek `docker compose exec backend ls -la /app/data`.

## Development

```bash
bun run dev        # backend :3001, vite :3002, reverse proxy :7777
bun run build      # frontend + backend + proxy
cd apps/backend && go test ./...
```

Vite mem-proxy `/api` ke backend, jadi frontend selalu memakai path relatif.
