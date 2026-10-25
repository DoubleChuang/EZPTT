// Package screen recognises the PTT terminal screens and decides what the
// client has to type next.
//
// It is deliberately pure: no network, no logging, no clock. Every rule is a
// substring of the Big5-decoded screen, so a recorded login transcript can be
// replayed in a unit test without touching ptt.cc.
package screen

import (
	"regexp"
	"strings"
	"time"
)

// ansiEscape matches the colour and cursor sequences PTT mixes into the text
// ("\x1b[1m密碼正確\x1b[m"). Matching has to happen on the stripped screen,
// otherwise a keyword split by a colour change never matches.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// Normalise removes ANSI escape sequences.
func Normalise(text string) string {
	return ansiEscape.ReplaceAllString(text, "")
}

// Action is what the session loop does after a screen has been recognised.
type Action int

const (
	// Ignore means the screen is not interesting; keep reading.
	Ignore Action = iota
	// Credentials means "type the account name, wait, then the password".
	Credentials
	// AnyKey sends a bare carriage return.
	AnyKey
	// Password sends the password again (kicking the duplicate session).
	Password
	// Confirm sends "y".
	Confirm
	// Quit sends "q".
	Quit
	// LoggedIn means the main menu is on screen: the login succeeded.
	LoggedIn
	// Failed means the login cannot succeed; Reason says why.
	Failed
)

func (a Action) String() string {
	switch a {
	case Ignore:
		return "ignore"
	case Credentials:
		return "credentials"
	case AnyKey:
		return "any-key"
	case Password:
		return "password"
	case Confirm:
		return "confirm"
	case Quit:
		return "quit"
	case LoggedIn:
		return "logged-in"
	case Failed:
		return "failed"
	}
	return "unknown"
}

// Rule maps a screen fragment to an action.
type Rule struct {
	// Name is stable and used in logs and tests.
	Name string
	// Match is looked for anywhere in the decoded screen.
	Match string
	// Action is what to do once Match is found.
	Action Action
	// Delay is waited before the action is executed; PTT needs a moment to
	// accept input after printing these screens.
	Delay time.Duration
	// Reason is the operator-facing message, only for Failed.
	Reason string
}

// rules is ordered: the first match wins, so terminal states come before the
// prompts they can be confused with ("密碼不對" before "密碼正確").
var rules = []Rule{
	{Name: "overloaded", Match: "系統過載", Action: Failed, Reason: "PTT 系統過載"},
	{Name: "busy", Match: "系統忙碌", Action: Failed, Reason: "PTT 系統忙碌"},
	{Name: "wrong-password", Match: "密碼不對", Action: Failed, Reason: "密碼不對"},
	{Name: "auth-failed", Match: "帳號認證失敗", Action: Failed, Reason: "帳號認證失敗"},
	{Name: "not-certified", Match: "尚未通過認證", Action: Failed, Reason: "帳號尚未通過認證"},

	{Name: "duplicate-login", Match: "您想刪除其他重複登入", Action: Password, Delay: time.Second},
	{Name: "delete-failed-attempts", Match: "您要刪除以上錯誤嘗試", Action: Confirm},
	{Name: "unfinished-post", Match: "您有一篇文章尚未完成", Action: Quit},

	{Name: "login-prompt", Match: "請輸入代號", Action: Credentials, Delay: time.Second},
	{Name: "password-accepted", Match: "密碼正確", Action: AnyKey, Delay: 2 * time.Second},
	{Name: "press-any-key", Match: "請按任意鍵繼續", Action: AnyKey, Delay: time.Second},
	{Name: "logging-in", Match: "登入中，請稍候", Action: AnyKey, Delay: 2 * time.Second},

	// The main menu is the only honest evidence that the credentials worked.
	{Name: "main-menu", Match: "主功能表", Action: LoggedIn},
}

// Rules returns a copy of the ordered rule table.
func Rules() []Rule {
	out := make([]Rule, len(rules))
	copy(out, rules)
	return out
}

// Classify returns the first rule matching text. The boolean reports whether
// anything matched; an unmatched screen yields Ignore.
func Classify(text string) (Rule, bool) {
	plain := Normalise(text)
	for _, r := range rules {
		if strings.Contains(plain, r.Match) {
			return r, true
		}
	}
	return Rule{Name: "unrecognised", Action: Ignore}, false
}

// Summary trims a screen for logging: collapsed whitespace, at most limit runes.
// It never returns raw control sequences, so a screen dump stays readable in a
// log file.
func Summary(text string, limit int) string {
	fields := strings.Fields(Normalise(text))
	joined := strings.Join(fields, " ")
	runes := []rune(joined)
	if limit > 0 && len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return joined
}
