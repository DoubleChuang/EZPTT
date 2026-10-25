package accounts

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := strings.Join([]string{
		"# 註解",
		"alice,secret1",
		"",
		"  bob , secret2 ",
		"carol",          // 缺密碼
		"dave,",          // 空密碼
		"alice,secret99", // 重複
	}, "\n")

	got, warnings, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d accounts, want 2: %+v", len(got), got)
	}
	if got[0] != (Account{User: "alice", Password: "secret1"}) {
		t.Fatalf("first account = %+v", got[0])
	}
	if got[1] != (Account{User: "bob", Password: "secret2"}) {
		t.Fatalf("whitespace was not trimmed: %+v", got[1])
	}
	if len(warnings) != 3 {
		t.Fatalf("warnings = %v, want 3", warnings)
	}
}

// A row with fewer columns used to panic with index out of range.
func TestParseShortRowDoesNotPanic(t *testing.T) {
	got, warnings, err := Parse(strings.NewReader("onlyuser\nalice,pw\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("accounts = %+v", got)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestParseEmpty(t *testing.T) {
	if _, _, err := Parse(strings.NewReader("# 只有註解\n\n")); err == nil {
		t.Fatal("expected an error for a file without accounts")
	}
}

func TestAccountStringHidesPassword(t *testing.T) {
	a := Account{User: "alice", Password: "hunter2"}
	if strings.Contains(a.String(), "hunter2") {
		t.Fatalf("String() leaks the password: %s", a)
	}
}

func TestLoadWarnsAboutPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "PttConfig.csv")
	if err := os.WriteFile(path, []byte("alice,secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	list, warnings, err := Load(path, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("accounts = %+v", list)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want plaintext + permissions", warnings)
	}
	joined := strings.Join(warnings, " | ")
	if !strings.Contains(joined, "chmod 600") || !strings.Contains(joined, "明文") {
		t.Fatalf("warnings = %v", warnings)
	}
}

// An encrypted file must not produce plaintext warnings.
func TestLoadEncryptedHasNoPlaintextWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "PttConfig.csv.enc")
	if err := os.WriteFile(path, []byte("alice,secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	decode := func(raw []byte) ([]byte, bool, error) { return raw, true, nil }

	list, warnings, err := Load(path, decode)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(list) != 1 || list[0].User != "alice" {
		t.Fatalf("accounts = %+v", list)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none for an encrypted file", warnings)
	}
}

// A decoder error (wrong key) must surface, not be swallowed.
func TestLoadDecoderError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "PttConfig.csv.enc")
	if err := os.WriteFile(path, []byte("EZPTT1garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	decode := func([]byte) ([]byte, bool, error) {
		return nil, true, errors.New("解密失敗")
	}
	if _, _, err := Load(path, decode); err == nil || !strings.Contains(err.Error(), "解密失敗") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, _, err := Load(filepath.Join(t.TempDir(), "nope.csv"), nil)
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if errors.Is(err, os.ErrNotExist) == false {
		// the wrapping must keep the cause reachable
		t.Fatalf("error does not wrap os.ErrNotExist: %v", err)
	}
}
