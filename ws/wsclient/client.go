package wsclient

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
)

//Big5toUTF8 轉換Big5編碼成UTF8編碼
func Big5toUTF8(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), traditionalchinese.Big5.NewDecoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

type WsClient struct {
	Headers http.Header
	URL     *url.URL
	RawConn *websocket.Conn
	mu      sync.Mutex
}

func NewWsClient(URL string, headers http.Header) (*WsClient, error) {

	u, err := url.Parse(URL)
	if err != nil {
		return nil, err
	}

	if headers == nil {
		headers = http.Header{
			"Origin": {"https://term.ptt.cc"},
		}
	}

	return &WsClient{
		Headers: headers,
		URL:     u,
	}, nil
}

func (c *WsClient) Conn() (*http.Response, error) {
	var (
		res *http.Response
		err error
	)
	// ignore certificate signed by unknown authority
	config := tls.Config{InsecureSkipVerify: true}
	dialer := websocket.Dialer{
		TLSClientConfig: &config,
	}
	
	
	c.RawConn, res, err = dialer.Dial(c.URL.String(), c.Headers)	
	if err != nil {
		status := ""
		if res != nil {
			status = res.Status
		}
		log.Fatal("dial:", err, " ", status)
		return res, err
	}

	return res, nil
}

func (c *WsClient) Close() error {
	if c.RawConn == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.RawConn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)

	if err != nil {
		log.Println("write close:", err)
		return err
	}

	err = c.RawConn.Close()
	c.RawConn = nil

	return err
}

func (c *WsClient) Read() ([]byte, error) {
	_, message, err := c.RawConn.ReadMessage()
	if err != nil {
		log.Println("read:", err)
		return []byte{}, err
	}

	m, err := Big5toUTF8(message)
	if err != nil {
		log.Println(err)
		log.Printf("recv: %s", m)
		return []byte{}, err
	}

	return m, nil
}

func (c *WsClient) Write(data []byte) error {
	if c.RawConn == nil {
		return errors.New("Disconnect")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.RawConn.WriteMessage(websocket.TextMessage, append(data, []byte("\r\n")...))
	if err != nil {
		log.Println(err)
		return err
	}

	return nil
}

func (c *WsClient) WriteBinary(data []byte) error {
	if c.RawConn == nil {
		return errors.New("Disconnect")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.RawConn.WriteMessage(websocket.BinaryMessage, append(data, []byte("\r\n")...))
	if err != nil {
		log.Println(err)
		return err
	}

	return nil
}
