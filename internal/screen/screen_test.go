package screen

import (
	"strings"
	"testing"
	"time"
)

// Real screens as they arrive after Big5 decoding from term.ptt.cc.
const (
	screenLoginPrompt = "批踢踢實業坊 歡迎您\r\n請輸入代號，或以 guest 參觀，或以 new 註冊："
	screenPasswordOK  = "密碼正確！ 開始登入系統..."
	screenLoggingIn   = "登入中，請稍候..."
	screenPressAnyKey = "請按任意鍵繼續"
	screenMainMenu    = "【主功能表】                      看板《AllTogether》"
	screenDuplicate   = "您想刪除其他重複登入的連線嗎？ [Y/n]"
	screenWrongPasswd = "密碼不對"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		screen string
		want   Action
		reason string
	}{
		{"login prompt asks for credentials", screenLoginPrompt, Credentials, ""},
		{"accepted password needs a key", screenPasswordOK, AnyKey, ""},
		{"logging in needs a key after a while", screenLoggingIn, AnyKey, ""},
		{"press any key", screenPressAnyKey, AnyKey, ""},
		{"main menu means logged in", screenMainMenu, LoggedIn, ""},
		{"duplicate login resends the password", screenDuplicate, Password, ""},
		{"wrong password is terminal", screenWrongPasswd, Failed, "密碼不對"},
		{"overload is terminal", "系統過載，請稍後再試", Failed, "PTT 系統過載"},
		{"unknown screen is ignored", "歡迎來到批踢踢", Ignore, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule, matched := Classify(tc.screen)
			if rule.Action != tc.want {
				t.Fatalf("Classify(%q) = %v, want %v", tc.screen, rule.Action, tc.want)
			}
			if tc.want == Ignore && matched {
				t.Fatalf("unknown screen must not report a match")
			}
			if tc.reason != "" && rule.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q", rule.Reason, tc.reason)
			}
		})
	}
}

// 密碼不對 must never be read as 密碼正確; the ordering of the table is the fix.
func TestWrongPasswordIsNotAccepted(t *testing.T) {
	rule, _ := Classify("密碼不對，請重新輸入")
	if rule.Action != Failed {
		t.Fatalf("wrong password classified as %v (%s)", rule.Action, rule.Name)
	}
}

// PTT paints the screen: a keyword can be split by colour codes, which is why
// matching runs on the normalised text.
func TestClassifyIgnoresAnsiColourCodes(t *testing.T) {
	painted := "\x1b[1m請輸入代號\x1b[m，或以 guest 參觀："
	if rule, matched := Classify(painted); !matched || rule.Action != Credentials {
		t.Fatalf("painted prompt classified as %v (matched=%v)", rule.Action, matched)
	}

	paintedPassword := "\x1b[33;1m密碼正確\x1b[m！ 開始登入系統..."
	if rule, matched := Classify(paintedPassword); !matched || rule.Action != AnyKey {
		t.Fatalf("painted password screen classified as %v (matched=%v)", rule.Action, matched)
	}

	if rule, _ := Classify("\x1b[1;30m密碼不對\x1b[m"); rule.Action != Failed {
		t.Fatalf("painted failure classified as %v", rule.Action)
	}
}

func TestSummaryStripsAnsi(t *testing.T) {
	if got := Summary("\x1b[1m批踢踢\x1b[m 實業坊", 100); got != "批踢踢 實業坊" {
		t.Fatalf("Summary = %q", got)
	}
}

func TestDelaysAreSane(t *testing.T) {
	for _, r := range Rules() {
		if r.Delay < 0 {
			t.Fatalf("%s has a negative delay", r.Name)
		}
		if r.Delay > 10*time.Second {
			t.Fatalf("%s waits %s, too long", r.Name, r.Delay)
		}
	}
}

func TestSummary(t *testing.T) {
	got := Summary("  多行\r\n畫面   有空白  ", 100)
	if got != "多行 畫面 有空白" {
		t.Fatalf("Summary = %q", got)
	}
	long := strings.Repeat("字", 20)
	if got := Summary(long, 5); got != "字字字字字…" {
		t.Fatalf("Summary truncation = %q", got)
	}
}
