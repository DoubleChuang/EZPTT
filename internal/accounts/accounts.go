// Package accounts loads the PTT account list from a CSV file.
//
// The file is one account per line, `username,password`; blank lines and lines
// starting with '#' are ignored so the file can carry comments.
package accounts

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Account is one PTT login.
type Account struct {
	User     string
	Password string
}

// String keeps the password out of logs and error messages.
func (a Account) String() string { return a.User }

// Parse reads accounts from r. It returns the accounts plus non-fatal warnings
// (duplicate users, skipped rows) so the operator can see what was ignored.
func Parse(r io.Reader) ([]Account, []string, error) {
	reader := csv.NewReader(r)
	reader.Comment = '#'
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	var (
		out      []Account
		warnings []string
		seen     = map[string]struct{}{}
		line     int
	)
	for {
		line++
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, warnings, fmt.Errorf("第 %d 行解析失敗: %w", line, err)
		}
		if len(row) == 0 {
			continue
		}
		user := strings.TrimSpace(row[0])
		if user == "" {
			continue
		}
		if len(row) < 2 {
			warnings = append(warnings, fmt.Sprintf("第 %d 行只有帳號沒有密碼，已跳過 (%s)", line, user))
			continue
		}
		password := strings.TrimSpace(row[1])
		if password == "" {
			warnings = append(warnings, fmt.Sprintf("第 %d 行密碼為空，已跳過 (%s)", line, user))
			continue
		}
		if _, dup := seen[user]; dup {
			warnings = append(warnings, fmt.Sprintf("第 %d 行帳號重複，已跳過 (%s)", line, user))
			continue
		}
		seen[user] = struct{}{}
		out = append(out, Account{User: user, Password: password})
	}
	if len(out) == 0 {
		return nil, warnings, errors.New("帳號檔沒有任何有效帳號")
	}
	return out, warnings, nil
}

// Load opens path and parses it. It warns when the file is readable by other
// users, because it holds plaintext passwords.
func Load(path string) ([]Account, []string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("讀取帳號檔 %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("帳號檔 %s 是目錄", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("開啟帳號檔 %s: %w", path, err)
	}
	defer f.Close()

	list, warnings, err := Parse(f)
	if err != nil {
		return nil, warnings, fmt.Errorf("%s: %w", path, err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		warnings = append(warnings, fmt.Sprintf("帳號檔 %s 權限是 %#o，內含明文密碼，建議 chmod 600", path, mode))
	}
	return list, warnings, nil
}
