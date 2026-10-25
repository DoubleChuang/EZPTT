// Package ws is the PTT websocket adapter (the protocol term.ptt.cc speaks).
//
// It implements the session.Conn port: screens arrive Big5-encoded on binary
// frames, and every keystroke is one binary frame terminated with CRLF.
package ws

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/text/encoding/traditionalchinese"

	"github.com/DoubleChuang/EZPTT/internal/session"
)

// Options configures the dialer.
type Options struct {
	// URL is the websocket endpoint, e.g. wss://ws.ptt.cc/bbs.
	URL string
	// Origin is sent as the Origin header; term.ptt.cc requires its own URL.
	Origin string
	// Header carries extra handshake headers.
	Header http.Header
	// InsecureSkipVerify disables certificate verification. Off by default:
	// the connection carries the account password.
	InsecureSkipVerify bool
	// ReadTimeout bounds the wait for one screen.
	ReadTimeout time.Duration
	// HandshakeTimeout bounds the dial.
	HandshakeTimeout time.Duration
}

// Dialer opens PTT connections.
type Dialer struct {
	opts Options
}

// NewDialer returns a dialer for the given options.
func NewDialer(opts Options) *Dialer {
	if opts.HandshakeTimeout <= 0 {
		opts.HandshakeTimeout = 15 * time.Second
	}
	return &Dialer{opts: opts}
}

// Dial implements session.Dialer.
func (d *Dialer) Dial(ctx context.Context) (session.Conn, error) {
	u, err := url.Parse(d.opts.URL)
	if err != nil {
		return nil, fmt.Errorf("解析 WS_URL %q: %w", d.opts.URL, err)
	}

	header := http.Header{}
	for key, values := range d.opts.Header {
		header[key] = append([]string(nil), values...)
	}
	if d.opts.Origin != "" {
		header.Set("Origin", d.opts.Origin)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: d.opts.HandshakeTimeout,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// Opt-in only: EZPTT_INSECURE_TLS=true, for a host without CA certs.
			InsecureSkipVerify: d.opts.InsecureSkipVerify, //nolint:gosec // deliberate, documented escape hatch
		},
	}

	conn, resp, err := dialer.DialContext(ctx, u.String(), header)
	if err != nil {
		status := "沒有回應"
		if resp != nil {
			status = resp.Status
		}
		return nil, fmt.Errorf("連線 %s 失敗: %w (%s)", u.Redacted(), err, status)
	}
	return &client{conn: conn, readTimeout: d.opts.ReadTimeout}, nil
}

// client is one live PTT connection.
type client struct {
	conn        *websocket.Conn
	readTimeout time.Duration

	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

func (c *client) Read() ([]byte, error) {
	if c.readTimeout > 0 {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	}
	_, payload, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return DecodeBig5(payload), nil
}

func (c *client) Write(p []byte) error {
	msg := make([]byte, 0, len(p)+2)
	msg = append(msg, p...)
	msg = append(msg, '\r', '\n')

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
		return fmt.Errorf("寫入 websocket: %w", err)
	}
	return nil
}

// Close is idempotent and never panics, including on a half-open connection.
func (c *client) Close() error {
	c.closeOnce.Do(func() {
		deadline := time.Now().Add(time.Second)
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			deadline,
		)
		c.closeErr = c.conn.Close()
	})
	return c.closeErr
}

// DecodeBig5 converts PTT's Big5 payload to UTF-8, passing undecodable bytes
// through instead of failing the read: one malformed control frame must not
// abort a login.
func DecodeBig5(payload []byte) []byte {
	decoded, err := traditionalchinese.Big5.NewDecoder().Bytes(payload)
	if err != nil {
		return payload
	}
	return decoded
}
