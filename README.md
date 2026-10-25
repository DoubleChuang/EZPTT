# EZPTT

定時登入 PTT，讓多個帳號保持活躍。一個 binary、一支 cron，跑在 Raspberry Pi 或任何容器裡都行。

```
$ ezptt -once
level=INFO msg="載入帳號" count=2 file=/data/PttConfig.csv version=v2.0.0
level=INFO msg="登入成功" account=alice took=4.213s
level=ERROR msg="登入失敗" account=bob err="密碼不對" last_screen="..."
level=INFO msg="本輪結果" ok=1 failed=1 total=2
ERROR: 1/2 個帳號登入失敗
```

## 功能

- 多帳號，CSV 一行一個，可加註解
- cron 排程常駐，`-once` 可手動跑一輪（失敗時 exit 1，給 systemd/cron 判斷）
- `-probe` 不需要帳密就能檢查 PTT 連線與畫面辨識
- 登入成功以「看到主功能表」為準，不是「連線建立」就算成功
- 每次登入都乾淨關閉連線，不會漏 socket 或 goroutine
- 結構化 log（`log/slog`），密碼永不進 log
- 多平台映像：`linux/amd64`、`linux/arm64`、`linux/arm/v7`

## 快速開始

### Docker（建議）

```console
mkdir -p ~/ezptt/data && cd ~/ezptt
curl -fsSLO https://raw.githubusercontent.com/DoubleChuang/EZPTT/master/docker-compose.yml
cp .env.example ezptt.env          # 或 docker-compose.yml 同層自己寫一份

# 帳號檔：一行一個帳號，權限務必 600
printf 'your_ptt_id,your_password\n' > data/PttConfig.csv
chmod 600 data/PttConfig.csv

docker compose up -d
docker compose logs -f
```

### 本機執行檔

```console
go build -o ezptt ./cmd/ezptt
printf 'your_ptt_id,your_password\n' > PttConfig.csv
chmod 600 PttConfig.csv
./ezptt -probe        # 先確認連得上 PTT
./ezptt -once         # 手動登入一輪
./ezptt               # 依 CRON_SPEC 常駐
```

## 設定

設定來源優先序：命令列旗標 > 環境變數（`EZPTT_` 前綴）> 設定檔（`-config`，未指定時讀 `./.env`）> 預設值。

| 環境變數 | 預設 | 說明 |
| --- | --- | --- |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error`（也接受舊版 `0`-`3`） |
| `CRON_SPEC` | `5 3 * * *` | 5 欄（分 時 日 月 週）或 6 欄（秒 分 時 日 月 週），每天 03:05 |
| `ACCOUNTS_FILE` | `PttConfig.csv` | 帳號 CSV；容器內預設 `/data/PttConfig.csv` |
| `MAX_CONCURRENCY` | `4` | 同時登入幾個帳號（1-16） |
| `LOGIN_TIMEOUT` | `90s` | 單一帳號的登入逾時 |
| `READ_TIMEOUT` | `30s` | 等待下一個畫面的逾時 |
| `HOLD` | `0` | 登入成功後保持連線的時間，`0` = 登入完就斷線 |
| `INSECURE_TLS` | `false` | 跳過 TLS 憑證驗證，只在主機缺 CA 憑證時使用 |
| `RUN_ON_START` | `false` | 啟動後先跑一輪再照排程；容器重啟就能立即驗證帳號 |
| `WS_URL` | `wss://ws.ptt.cc/bbs` | PTT websocket 端點 |
| `ORIGIN` | `https://term.ptt.cc` | 連線帶的 Origin header |

`CRON_SPEC` 用**容器/主機的本地時間**，映像預設 `TZ=Asia/Taipei`；自己跑 binary 時請確認 `TZ`。

帳號檔每次執行前都會重讀，換帳號不用重建容器。

## 部署到 Raspberry Pi

Pi 是 aarch64（64 位元）就直接用 `linux/arm64` 映像：

```console
ssh pi@<host>
mkdir -p ~/docker/ezptt/data && cd ~/docker/ezptt
# 放好 docker-compose.yml、ezptt.env、data/PttConfig.csv（chmod 600）
docker compose up -d
docker compose logs -f
```

`docker-compose.yml` 預設用 `user: "${PUID:-1000}:${PGID:-1000}"` 執行，因為帳號檔是 `0600` 且屬於主機使用者；映像本身預設是 uid 10001，若直接 `docker run` 就必須 `chown 10001:10001 data/PttConfig.csv`。

`ezptt.env` 設 `RUN_ON_START=true` 時，`docker compose up -d` 或 `restart` 會立刻登入一輪，log 就能看到結果，不必等到 cron 時間。

`restart: unless-stopped` 讓它在 PTT 網路抖動、主機重開後自己回來；log 有 `max-size` 上限，不會把 SD 卡寫滿。

## 開發

```console
make test     # go test ./...
make race     # go test -race ./...
make lint     # gofmt 檢查 + go vet（CI 也跑同樣的）
make build    # bin/ezptt
```

CI（GitHub Actions）：`gofmt` / `go vet` / `go test -race` / `golangci-lint`，推送 master 會建多平台映像推到 Docker Hub 與 GHCR；打 `v*` tag 由 goreleaser 出跨平台壓縮檔。

## 架構

```
cmd/ezptt/            組裝：旗標、logger、signal、cron
internal/screen/      domain：PTT 畫面 → 動作的規則表（純函式，可離線測試）
internal/session/     use case：登入狀態機，只依賴 Conn/Dialer 介面
internal/transport/ws adapter：gorilla websocket、Big5 解碼、TLS
internal/accounts/    帳號 CSV 解析與權限檢查
internal/config/      設定解析（env > 檔案 > 預設）
internal/logx/        slog 建立
```

依賴方向往內：`session` 不知道 websocket 的存在，所以整個登入流程可以在單元測試裡用腳本化的假連線跑完（含密碼錯誤、重複登入、逾時），不需要連 PTT。

要支援新的登入畫面，只改 `internal/screen/screen.go` 的規則表，並在 `screen_test.go` 加一個 case。

## 安全性

- `PttConfig.csv` 存的是明文密碼：`chmod 600`，不要進 git（`.gitignore` 已排除）。程式在權限過寬時會警告。
- TLS 憑證預設驗證；映像已安裝 `ca-certificates`。`INSECURE_TLS=true` 會讓帳密暴露在中間人攻擊下。
- 容器以非 root（uid 10001）執行，帳號檔以唯讀方式掛載。

## 從 1.x 升級

- 設定檔、`PttConfig.csv` 格式不變，`./.env` 仍會自動載入。
- `CRON_SPEC` 6 欄（含秒）仍然可用；舊的 `0 5 3 * * *` 等於新的 `5 3 * * *`。
- Makefile 不再產生 `bin/{platform}/EZPTT`；跨平台發布改由 goreleaser 與容器映像負責。
- 帳號檔預設路徑從「執行檔所在目錄」改為目前工作目錄，容器內是 `/data/PttConfig.csv`。
