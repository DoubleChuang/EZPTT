package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DoubleChuang/EZPTT/internal/vault"
)

// Environment variables that hold the account-file key.
const (
	configKeyEnv     = "EZPTT_ENCRYPTION_KEY"
	configKeyFileEnv = "EZPTT_KEY_FILE"
)

// Toolchain for the encrypted account file: keygen → encrypt → run.
//
// The key is never written next to the accounts: it belongs in a 0600 file
// outside the data directory (or in the environment), so a copy of
// PttConfig.csv.enc on its own is useless.

func keygenCommand(argv []string) error {
	fs := flag.NewFlagSet("ezptt keygen", flag.ContinueOnError)
	out := fs.String("out", "", "金鑰寫入這個檔案（0600）；不給就印到 stdout")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), "用法: ezptt keygen [-out ~/.config/ezptt/key]\n\n產生一把 base64 的 AES-256 金鑰。\n")
	}
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	encoded, err := vault.GenerateKey()
	if err != nil {
		return err
	}

	if *out == "" {
		fmt.Println(encoded)
		fmt.Fprintln(os.Stderr, "警告：金鑰只會顯示一次，請立刻存進密碼管理器；遺失就解不開 PttConfig.csv.enc。")
		return nil
	}
	if _, err := os.Stat(*out); err == nil {
		return fmt.Errorf("%s 已存在，不覆蓋既有金鑰", *out)
	}
	if dir := filepath.Dir(*out); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("建立目錄 %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(*out, []byte(encoded+"\n"), 0o600); err != nil {
		return fmt.Errorf("寫入金鑰檔 %s: %w", *out, err)
	}
	fmt.Printf("金鑰已寫入 %s（0600）。請另存一份到密碼管理器。\n", *out)
	return nil
}

func encryptCommand(argv []string) error {
	fs := flag.NewFlagSet("ezptt encrypt", flag.ContinueOnError)
	in := fs.String("in", "PttConfig.csv", "明文帳號檔")
	out := fs.String("out", "", "輸出檔，預設 <in>.enc")
	keyFile := fs.String("key-file", "", "金鑰檔（預設讀 EZPTT_ENCRYPTION_KEY，其次 EZPTT_KEY_FILE）")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), "用法: ezptt encrypt [-in PttConfig.csv] [-out PttConfig.csv.enc] [-key-file ~/.config/ezptt/key]\n")
	}
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	target := *out
	if target == "" {
		target = *in + ".enc"
	}

	key, err := loadKey(*keyFile)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("讀取 %s: %w", *in, err)
	}
	if vault.IsEncrypted(raw) {
		return fmt.Errorf("%s 已經是加密檔", *in)
	}
	sealed, err := key.Encrypt(raw)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, sealed, 0o600); err != nil {
		return fmt.Errorf("寫入 %s: %w", target, err)
	}
	fmt.Printf("已加密 %s → %s\n", *in, target)
	fmt.Printf("確認程式讀得到之後，把明文 %s 刪掉。\n", *in)
	return nil
}

func decryptCommand(argv []string) error {
	fs := flag.NewFlagSet("ezptt decrypt", flag.ContinueOnError)
	in := fs.String("in", "", "加密帳號檔")
	out := fs.String("out", "", "輸出檔；不給就印到 stdout")
	keyFile := fs.String("key-file", "", "金鑰檔（預設讀 EZPTT_ENCRYPTION_KEY，其次 EZPTT_KEY_FILE）")
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), "用法: ezptt decrypt -in PttConfig.csv.enc [-out PttConfig.csv] [-key-file …]\n")
	}
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *in == "" {
		return errors.New("需要 -in <加密檔>")
	}

	key, err := loadKey(*keyFile)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*in)
	if err != nil {
		return fmt.Errorf("讀取 %s: %w", *in, err)
	}
	plain, err := key.Decrypt(raw)
	if err != nil {
		return err
	}
	if *out == "" {
		_, _ = os.Stdout.Write(plain)
		return nil
	}
	if err := os.WriteFile(*out, plain, 0o600); err != nil {
		return fmt.Errorf("寫入 %s: %w", *out, err)
	}
	fmt.Printf("已解密 %s → %s（0600）\n", *in, *out)
	return nil
}

// loadKey resolves the key: flag > EZPTT_ENCRYPTION_KEY > EZPTT_KEY_FILE.
func loadKey(keyFileFlag string) (*vault.Key, error) {
	if encoded := strings.TrimSpace(os.Getenv(configKeyEnv)); encoded != "" {
		key, err := vault.NewKey(encoded)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", configKeyEnv, err)
		}
		return key, nil
	}
	path := keyFileFlag
	if path == "" {
		path = strings.TrimSpace(os.Getenv(configKeyFileEnv))
	}
	if path == "" {
		return nil, fmt.Errorf("沒有金鑰：設定 %s、%s，或加 -key-file", configKeyEnv, configKeyFileEnv)
	}
	encoded, err := vault.ReadKeyFile(path)
	if err != nil {
		return nil, err
	}
	key, err := vault.NewKey(encoded)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return key, nil
}
