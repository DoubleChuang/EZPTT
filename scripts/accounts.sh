#!/usr/bin/env bash
# 編輯加密的帳號檔：解密到 0700 暫存目錄 → $EDITOR → 驗證 → 重新加密 → 抹除明文
#
# 用法:
#   ./accounts.sh [data/PttConfig.csv.enc]
#
# 環境變數:
#   EDITOR            編輯器（預設 vi）
#   EZPTT_BIN         本機 ezptt 執行檔；沒設就用容器
#   EZPTT_IMAGE       容器映像（預設 doublehub/ezptt:latest）
#   EZPTT_KEY_DIR     金鑰目錄（預設 ~/.config/ezptt）
#   EZPTT_KEY_NAME    金鑰檔名（預設 key）
#
# 安全設計:
#   - 明文只存在 mktemp -d 的 0700 目錄，離開時 shred -u
#   - 新檔先寫到暫存、解密回讀比對成功才原子替換
#   - 舊密文留一份 .bak（是密文，不影響安全）
set -euo pipefail

ENC="${1:-data/PttConfig.csv.enc}"
EDITOR_BIN="${EDITOR:-vi}"
IMAGE="${EZPTT_IMAGE:-doublehub/ezptt:latest}"
KEY_DIR="${EZPTT_KEY_DIR:-$HOME/.config/ezptt}"
KEY_NAME="${EZPTT_KEY_NAME:-key}"
BIN="${EZPTT_BIN:-}"

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

[[ -f "$ENC" ]] || die "找不到 $ENC"
[[ -f "$KEY_DIR/$KEY_NAME" ]] || die "找不到金鑰 $KEY_DIR/$KEY_NAME（先跑 ezptt keygen）"

ENC_ABS="$(cd "$(dirname "$ENC")" && pwd)/$(basename "$ENC")"
ENC_DIR="$(dirname "$ENC_ABS")"
ENC_BASE="$(basename "$ENC_ABS")"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/ezptt-accounts.XXXXXX")"
chmod 700 "$WORK"
PLAIN="$WORK/PttConfig.csv"

cleanup() {
    shred -u "$PLAIN" 2>/dev/null || rm -f "$PLAIN" 2>/dev/null || true
    rm -f "$WORK/check.csv" "$WORK/new.enc" 2>/dev/null || true
    rmdir "$WORK" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# 有本機 binary 就直接跑，否則用容器（路徑會掛進 /data、/keys、/work）
ez() {
    if [[ -n "$BIN" ]]; then
        "$BIN" "$@"
    else
        # 用主機 uid 執行，才讀得到 0600 的金鑰與帳號檔（映像預設是 10001）
        docker run --rm -i \
            -u "$(id -u):$(id -g)" \
            -v "$WORK:/work" \
            -v "$ENC_DIR:/data:ro" \
            -v "$KEY_DIR:/keys:ro" \
            "$IMAGE" "$@"
    fi
}

echo "→ 解密 $ENC_ABS"
if [[ -n "$BIN" ]]; then
    ez decrypt -in "$ENC_ABS" -out "$PLAIN" -key-file "$KEY_DIR/$KEY_NAME" >/dev/null
else
    ez decrypt -in "/data/$ENC_BASE" -out /work/PttConfig.csv -key-file "/keys/$KEY_NAME" >/dev/null
fi
chmod 600 "$PLAIN"

BEFORE="$(sha256sum "$PLAIN" | cut -d' ' -f1)"
"$EDITOR_BIN" "$PLAIN"
AFTER="$(sha256sum "$PLAIN" | cut -d' ' -f1)"
if [[ "$BEFORE" == "$AFTER" ]]; then
    echo "沒有修改，結束。"
    exit 0
fi

echo "→ 加密新內容"
if [[ -n "$BIN" ]]; then
    ez encrypt -in "$PLAIN" -out "$WORK/new.enc" -key-file "$KEY_DIR/$KEY_NAME" >/dev/null
    ez decrypt -in "$WORK/new.enc" -out "$WORK/check.csv" -key-file "$KEY_DIR/$KEY_NAME" >/dev/null
else
    ez encrypt -in /work/PttConfig.csv -out /work/new.enc -key-file "/keys/$KEY_NAME" >/dev/null
    ez decrypt -in /work/new.enc -out /work/check.csv -key-file "/keys/$KEY_NAME" >/dev/null
fi

cmp -s "$PLAIN" "$WORK/check.csv" || die "新檔解密後與編輯內容不一致，未替換原檔（原檔未動）"

cp "$ENC_ABS" "$ENC_ABS.bak"
cp "$WORK/new.enc" "$ENC_ABS.tmp"
chmod 600 "$ENC_ABS.tmp"
mv "$ENC_ABS.tmp" "$ENC_ABS"
echo "✓ 已更新 $ENC_ABS（舊檔備份為 $(basename "$ENC_ABS").bak）"
echo "  重啟讓新帳號生效：docker compose restart（RUN_ON_START=true 會立刻跑一輪）"
