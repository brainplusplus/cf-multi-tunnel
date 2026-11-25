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

Saat publish, isinya sama seperti Cloudflare sendiri: pilih **subdomain + domain**
(domain dari zone yang sudah tersinkron), lalu **service URL**. Untuk tunnel `local`,
ingat connector berjalan di container sendiri, jadi `localhost` bukan host Anda —
pakai `http://host.docker.internal:8080` (panel menambahkan host-gateway otomatis)
atau nama container di network `cfmt-panel`.

**Panel exposure** — admin bisa mempublikasikan dashboard ini sendiri lewat tunnel
(**Settings → Panel exposure**). Formnya mengikuti alur yang sama: pilih **akun
(opsional)** untuk mempersempit daftar, pilih **tunnel**, lalu **subdomain + domain**.
Service URL tidak diisi sama sekali; panel selalu memakai `PANEL_INTERNAL_URL`
(mis. `http://reverse-proxy:7777`) karena container connector menjangkau dashboard
lewat network Docker `cfmt-panel`.

Hanya tunnel yang terpasang **di server ini** (`location=local`) yang bisa dipilih.
Connector di perangkat lain tidak bisa me-resolve `reverse-proxy`, dan mengarahkannya
ke `PUBLIC_URL` akan memutar balik lewat tunnel yang sedang dibuat — jadi backend
menolaknya dengan 422 pada `tunnel_id`. Exposure lama yang masih menempel di tunnel
remote tetap ditampilkan di Settings beserta peringatan, supaya tidak hilang diam-diam.

Exposure disimpan sebagai published app dengan `kind=panel`, jadi hostname-nya
**otomatis ikut masuk** ke ingress tunnel tersebut dan muncul di halaman
**Published apps** (dengan badge `panel`). Baris itu tidak bisa diubah/dimatikan/
dihapus dari daftar app — semuanya lewat Settings. Menghapus tunnel atau akun yang
sedang memuat exposure juga ditolak sampai exposure dipindah/dihapus dulu.

**Bahasa** — seluruh UI punya teks Indonesia dan Inggris; default Indonesia, dan
tombol ID/EN di sidebar menyimpan pilihan di `localStorage.language`. Halaman baru
wajib menambah `src/locales/<module>.json` dan memakai `useTranslation('<module>')`.

**Impersonation** — admin bisa memakai panel sebagai pengguna lain dari halaman
**Pengguna** (tombol *Masuk sebagai*). Sesi itu memakai token berumur pendek dengan
peran pengguna tersebut, bukan sesi admin yang disegarkan; menekan *Keluar dari mode
lihat* mengembalikan sesi admin semula. Kedua arah dicatat di audit
(`user.impersonate_start` menyebut admin asalnya, `user.impersonate_end`).

**DNS** — halaman **DNS** mengelola record domain yang zone-nya sudah tersinkron.
Pilih **akun** (opsional) lalu **domain**; tombol **Sinkron** menyegarkan daftar zone
dari setiap akun yang punya API token, lalu daftar record dimuat ulang. Record selalu
dibaca **langsung dari Cloudflare** (tidak ada cache record lokal), jadi yang terlihat
di panel adalah yang benar-benar aktif.

Semua tipe umum bisa dibuat/diubah/dihapus: `A`, `AAAA`, `CNAME`, `TXT`, `MX`, `NS`,
`SRV`, `CAA`. Form mengikuti tipe: `A`/`AAAA` meminta alamat IP, `CNAME`/`NS` hostname,
`MX`/`SRV` prioritas, `SRV` service + protokol + port + weight, `CAA` flags/tag/value,
dan hanya `A`/`AAAA`/`CNAME` yang bisa **Proxied**.

CNAME yang menunjuk ke `<tunnel-id>.cfargotunnel.com` ditampilkan bertipe **Tunnel**.
Kalau record itu dipakai **published app**, barisnya **terkunci** (ikon gembok): ubah
atau hapus dari halaman tunnel, supaya aturan ingress dan DNS tidak berbeda. Sync tidak
pernah menghapus record atau zone yang sudah ada.

## Permission token Cloudflare

Cloudflare sekarang memakai editor token berbasis **Policies**: satu policy punya satu
scope (Account / Zone / User), jadi izin yang dibutuhkan panel dipecah menjadi **dua
policy**. Alurnya:

1. Cloudflare Dashboard → My Profile → **API Tokens** → **Create Token**
2. Pilih **Create Custom Token** (template yang tersedia tidak punya akses tunnel)
3. **Token name**: bebas, mis. `cf-multi-tunnel`
4. **Policy 1 — Account.** Klik `+ Add new policy`, set tipe **Account**, lalu
   `Include → Specific account` → pilih akun pemilik tunnel. Di kotak
   **"Search for permission groups…"** cari dan centang:

   | Cari | Permission group | Centang |
   |---|---|---|
   | `cloudflare tunnel` | Cloudflare Tunnel | **Edit** |
   | `account settings` | Account Settings | **Read** |

5. **Policy 2 — Zone.** Editor sudah menyiapkan satu policy bernama
   **Edit Zone policy**; pakai itu (kalau tidak ada, klik `+ Add new policy` dan set
   tipe **Zone**). Lalu `Include → Specific zone` → pilih domain tujuan. Centang:

   | Cari | Permission group | Centang |
   |---|---|---|
   | `zone` | Zone | **Read** |
   | `dns` | DNS | **Edit** |

6. **Token expiration**: pilih sesuai kebijakan Anda. `No expiration` membuat panel
   terus jalan sampai token di-roll; tanggal tetap berarti harus diingat untuk diganti.
   **Client IP address filtering** biarkan kosong kecuali panel punya IP keluar tetap.
7. **Continue to summary**, lalu **cek summary sebelum** klik Create Token. Isinya
   harus memuat tepat empat baris ini:

   ```
   Cloudflare Tunnel: Edit
   Account Settings: Read
   Zone: Read
   DNS: Edit
   ```

   Tidak ada baris **Cloudflare Tunnel** = policy Account belum dibuat. Kalau sudah
   benar, baru Create Token dan copy (hanya ditampilkan sekali).

8. **Cek juga baris scope di atas tiap daftar izin.** Harus berbunyi
   `All accounts` → seharusnya `Specific account`, dan
   `All zones across all accounts` → seharusnya `Specific zone`. Kalau masih "All …",
   token tetap jalan tapi memberi `DNS: Edit` ke **semua** domain Anda — balik ke
   masing-masing policy dan ganti Include-nya.

Jangan dicentang (sering salah klik):

- **`All zones across all accounts` / `All accounts`** — ini opsi Include default, jadi
  mudah terlewat. Cakupannya seluruh akun dan seluruh domain.
- **Hanya policy Zone** — editor memang membuka dengan satu Zone policy, dan mudah
  berhenti di situ. Tanpa policy Account, panel tidak bisa membaca daftar tunnel
  sama sekali.
- **Apps** (grup *Account & Billing*) — deprecated, tidak ada hubungannya dengan tunnel.
- **Cloudflare One Connector: cloudflared** — mengatur connector yang dikelola
  Cloudflare Zero Trust, bukan konfigurasi tunnel. Panel menjalankan cloudflared
  sendiri, jadi tidak perlu.
- **Permission Zone lain** — Zone Settings, Access, Zone WAF, Custom Assets, Zone
  Versioning, DNS Settings, dan sejenisnya tidak pernah dipanggil panel. Setiap
  `Edit` tambahan hanya memperbesar dampak kalau token bocor.
- Izin **Account** di dalam **Zone policy** — tiap policy hanya satu scope, jadi
  editor menyembunyikannya. Itu sebabnya perlu dua policy.

Ringkasnya, yang dipakai panel:

| Policy | Permission | Dipakai untuk |
|---|---|---|
| Account | Cloudflare Tunnel · Edit | list/read/tulis ingress tunnel |
| Account | Account Settings · Read | list akun & verifikasi token |
| Zone | Zone · Read | cari zone pemilik hostname |
| Zone | DNS · Edit | buat/ubah CNAME ke tunnel |

**Pilih akun lewat dropdown, bukan mengetik ID.** Begitu token ditempel, panel
memverifikasinya (`POST /api/cf-accounts/probe`, tidak menyimpan apa pun) dan
mengambil daftar akun yang bisa dilihat token itu:

- token hanya melihat **satu** akun → akun tersebut langsung terpilih, ditampilkan
  sebagai nama + ID (tidak ada yang perlu dipilih)
- token melihat **beberapa** akun → muncul dropdown berisi nama akun; akun yang sudah
  ditambahkan dengan API token ditandai *already added* dan tidak bisa dipilih lagi
- token ditolak / kurang izin → pesan errornya muncul di bawah field, dan ID masih
  bisa diisi manual sebagai fallback

Account ID manual hanya diperlukan kalau Anda tahu persis ID-nya; nilainya ada di URL
dashboard (`dash.cloudflare.com/<account-id>`) atau sidebar Overview sebuah domain.

Panduan langkah demi langkah yang sama (termasuk tabel per-policy dan daftar
"jangan dicentang") juga tersedia langsung di dalam modal
**Accounts → Add API token** dan **Attach an API token**.

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
