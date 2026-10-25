package session

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DoubleChuang/EZPTT/internal/accounts"
)

const (
	screenLoginPrompt = "批踢踢實業坊\r\n請輸入代號，或以 guest 參觀："
	screenPasswordOK  = "密碼正確！ 開始登入系統..."
	screenMainMenu    = "【主功能表】 看板《AllTogether》"
	screenDuplicate   = "您想刪除其他重複登入的連線嗎？"
	screenUnknown     = "歡迎來到批踢踢"
)

// fakeConn replays scripted screens and records what the session typed.
type fakeConn struct {
	frames [][]byte
	next   int
	writes [][]byte
	closed bool
	mu     sync.Mutex
}

func (c *fakeConn) Read() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.next >= len(c.frames) {
		return nil, io.EOF
	}
	frame := c.frames[c.next]
	c.next++
	return frame, nil
}

func (c *fakeConn) Write(p []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, append([]byte(nil), p...))
	return nil
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeConn) typed() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.writes))
	for i, w := range c.writes {
		out[i] = string(w)
	}
	return out
}

type fakeDialer struct {
	mu    sync.Mutex
	conns []*fakeConn
	err   error
	dials int
}

func (d *fakeDialer) Dial(context.Context) (Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dials++
	if d.err != nil {
		return nil, d.err
	}
	if len(d.conns) == 0 {
		return nil, errors.New("no scripted connection")
	}
	c := d.conns[0]
	d.conns = d.conns[1:]
	return c, nil
}

func frames(list ...string) [][]byte {
	out := make([][]byte, len(list))
	for i, s := range list {
		out[i] = []byte(s)
	}
	return out
}

// instantSleep drops PTT's pacing delays so the tests run in milliseconds.
func instantSleep(context.Context, time.Duration) error { return nil }

func fastRunner(d Dialer, opts Options) *Runner {
	r := NewRunner(d, slog.New(slog.DiscardHandler), opts)
	r.sleep = instantSleep
	return r
}

func newTestRunner(t *testing.T, d Dialer) *Runner {
	t.Helper()
	return fastRunner(d, Options{
		LoginTimeout: 5 * time.Second,
		MaxScreens:   10,
		Concurrency:  2,
	})
}

func TestLoginSucceedsOnMainMenu(t *testing.T) {
	conn := &fakeConn{frames: frames(screenLoginPrompt, screenPasswordOK, screenMainMenu)}
	r := newTestRunner(t, &fakeDialer{conns: []*fakeConn{conn}})

	res := r.Login(context.Background(), accounts.Account{User: "alice", Password: "secret"})
	if !res.OK || res.Err != nil {
		t.Fatalf("Login = %+v", res)
	}
	if !conn.closed {
		t.Fatal("connection was not closed")
	}
	want := []string{"alice", "secret", ""}
	got := conn.typed()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("typed %q, want %q", got, want)
	}
}

// The old client reported success as soon as the websocket dialed, so a wrong
// password looked like a success. It must be a failure now.
func TestLoginWrongPasswordFails(t *testing.T) {
	conn := &fakeConn{frames: frames(screenLoginPrompt, "密碼不對")}
	r := newTestRunner(t, &fakeDialer{conns: []*fakeConn{conn}})

	res := r.Login(context.Background(), accounts.Account{User: "alice", Password: "nope"})
	if res.OK {
		t.Fatal("wrong password reported as success")
	}
	if !strings.Contains(res.Err.Error(), "密碼不對") {
		t.Fatalf("err = %v", res.Err)
	}
}

func TestLoginOverloadFails(t *testing.T) {
	conn := &fakeConn{frames: frames("系統過載，請稍後再試")}
	r := newTestRunner(t, &fakeDialer{conns: []*fakeConn{conn}})

	res := r.Login(context.Background(), accounts.Account{User: "alice", Password: "x"})
	if res.OK || !strings.Contains(res.Err.Error(), "系統過載") {
		t.Fatalf("res = %+v", res)
	}
}

func TestLoginDuplicateSessionResendsPassword(t *testing.T) {
	conn := &fakeConn{frames: frames(screenLoginPrompt, screenDuplicate, screenPasswordOK, screenMainMenu)}
	r := newTestRunner(t, &fakeDialer{conns: []*fakeConn{conn}})

	res := r.Login(context.Background(), accounts.Account{User: "alice", Password: "secret"})
	if !res.OK {
		t.Fatalf("Login = %+v", res)
	}
	got := conn.typed()
	if len(got) != 4 || got[2] != "secret" {
		t.Fatalf("typed %q, want the password again for the duplicate session", got)
	}
}

func TestLoginNeverReachingMenuIsAnError(t *testing.T) {
	conn := &fakeConn{frames: frames(screenLoginPrompt, screenUnknown, screenUnknown, screenUnknown)}
	r := fastRunner(&fakeDialer{conns: []*fakeConn{conn}}, Options{
		LoginTimeout: 5 * time.Second,
		MaxScreens:   3,
	})
	res := r.Login(context.Background(), accounts.Account{User: "alice", Password: "secret"})
	if res.OK {
		t.Fatal("login without a menu screen reported as success")
	}
	if !strings.Contains(res.Err.Error(), "登入未完成") {
		t.Fatalf("err = %v", res.Err)
	}
	if !conn.closed {
		t.Fatal("connection leaked on failure")
	}
}

func TestLoginDialFailure(t *testing.T) {
	r := newTestRunner(t, &fakeDialer{err: errors.New("no route to host")})
	res := r.Login(context.Background(), accounts.Account{User: "alice", Password: "secret"})
	if res.OK || !strings.Contains(res.Err.Error(), "連線失敗") {
		t.Fatalf("res = %+v", res)
	}
}

func TestLoginAllKeepsOrderAndIsolatesFailures(t *testing.T) {
	c1 := &fakeConn{frames: frames(screenLoginPrompt, screenPasswordOK, screenMainMenu)}
	c2 := &fakeConn{frames: frames(screenLoginPrompt, "密碼不對")}
	c3 := &fakeConn{frames: frames(screenLoginPrompt, screenPasswordOK, screenMainMenu)}
	r := newTestRunner(t, &fakeDialer{conns: []*fakeConn{c1, c2, c3}})

	list := []accounts.Account{
		{User: "a", Password: "1"},
		{User: "b", Password: "2"},
		{User: "c", Password: "3"},
	}
	results := r.LoginAll(context.Background(), list)
	if len(results) != 3 {
		t.Fatalf("got %d results", len(results))
	}
	// Results stay attached to the account they describe, whatever order the
	// dialer hands the connections out in.
	for i, want := range []string{"a", "b", "c"} {
		if results[i].Account != want {
			t.Fatalf("results[%d] = %q, want %q", i, results[i].Account, want)
		}
	}
	ok, failed := Summarise(results)
	if ok != 2 || failed != 1 {
		t.Fatalf("Summarise = %d ok, %d failed (results %+v)", ok, failed, results)
	}
	failures := 0
	for _, res := range results {
		if res.Err != nil && strings.Contains(res.Err.Error(), "密碼不對") {
			failures++
		}
	}
	if failures != 1 {
		t.Fatalf("expected exactly one password failure, got %d", failures)
	}
}

func TestLoginAllStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := newTestRunner(t, &fakeDialer{})
	results := r.LoginAll(ctx, []accounts.Account{{User: "a", Password: "1"}})
	if len(results) != 1 || results[0].Err == nil {
		t.Fatalf("results = %+v", results)
	}
}

// Passwords must never reach the log, even at debug level.
func TestLogsDoNotContainPassword(t *testing.T) {
	var buf bytes.Buffer
	conn := &fakeConn{frames: frames(screenLoginPrompt, screenPasswordOK, screenMainMenu)}
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := fastRunner(&fakeDialer{conns: []*fakeConn{conn}}, Options{LoginTimeout: 5 * time.Second})
	r.log = logger

	res := r.Login(context.Background(), accounts.Account{User: "alice", Password: "hunter2-secret"})
	if !res.OK {
		t.Fatalf("Login = %+v", res)
	}
	if strings.Contains(buf.String(), "hunter2") {
		t.Fatalf("log leaks the password:\n%s", buf.String())
	}
}
