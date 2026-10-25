package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/text/encoding/traditionalchinese"
)

type received struct {
	origin string
	msg    []byte
}

func testServer(t *testing.T) (*httptest.Server, chan received) {
	t.Helper()
	got := make(chan received, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		prompt, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte("請輸入代號"))
		if err != nil {
			return
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, prompt); err != nil {
			return
		}

		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		got <- received{origin: r.Header.Get("Origin"), msg: msg}
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestDialReadWriteClose(t *testing.T) {
	srv, got := testServer(t)
	dialer := NewDialer(Options{
		URL:         "ws" + strings.TrimPrefix(srv.URL, "http"),
		Origin:      "https://term.ptt.cc",
		ReadTimeout: 2 * time.Second,
	})

	conn, err := dialer.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	screen, err := conn.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(screen) != "請輸入代號" {
		t.Fatalf("Read = %q, Big5 was not decoded", screen)
	}

	if err := conn.Write([]byte("alice")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case msg := <-got:
		if string(msg.msg) != "alice\r\n" {
			t.Fatalf("server received %q, want the keystroke plus CRLF", msg.msg)
		}
		if msg.origin != "https://term.ptt.cc" {
			t.Fatalf("Origin = %q", msg.origin)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server never received the keystroke")
	}

	// Close must be idempotent (the old implementation returned early and never
	// closed at all, leaking one socket per login).
	if err := conn.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := conn.Write([]byte("x")); err == nil {
		t.Fatal("Write after Close must fail, not panic")
	}
}

func TestDialBadURL(t *testing.T) {
	if _, err := NewDialer(Options{URL: "://nope"}).Dial(context.Background()); err == nil {
		t.Fatal("expected an error for an unparseable URL")
	}
}

func TestDialUnreachable(t *testing.T) {
	// A port nobody listens on: the adapter must return an error, not exit.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	srv.Close()

	dialer := NewDialer(Options{URL: url, HandshakeTimeout: time.Second})
	if _, err := dialer.Dial(context.Background()); err == nil {
		t.Fatal("expected a connection error")
	}
}

func TestDecodeBig5KeepsUndecodableBytes(t *testing.T) {
	raw := []byte{0xff, 0xfe, 'o', 'k'}
	got := DecodeBig5(raw)
	if len(got) == 0 {
		t.Fatal("DecodeBig5 dropped the payload")
	}
}
