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
	list, warnings, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("accounts = %+v", list)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "chmod 600") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, _, err := Load(filepath.Join(t.TempDir(), "nope.csv"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if errors.Is(err, os.ErrNotExist) == false {
		// the wrapping must keep the cause reachable
		t.Fatalf("error does not wrap os.ErrNotExist: %v", err)
	}
}
