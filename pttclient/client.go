package pttclient

import (
	"bytes"
	"io/ioutil"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
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

//PTTClient ptt客戶端
type PTTClient struct {
	user string
	pswd string
	conn net.Conn
}

//NewPTTClient 創立一個ptt客戶端
func NewPTTClient(user, pswd string) *PTTClient {
	return &PTTClient{user, pswd, nil}
}

func (c *PTTClient) Write(s string, sec int) (int, error) {
	n, err := c.conn.Write([]byte(s + "\r\n"))
	if err != nil {
		return n, errors.Wrapf(err, "Write %s fail", s)
	}
	time.Sleep(time.Duration(sec) * time.Second)
	return n, err
}

//ByPassRead 將收下來的資訊直接丟棄
func (c *PTTClient) ByPassRead() error {
	var buf [8192]byte
	_, err := c.conn.Read(buf[:])
	if err != nil {
		return errors.Wrapf(err, "Read fail")
	}
	return err
}

//Login 登入PTT並回傳登錄狀態
func (c *PTTClient) Login() ([]byte, error) {
	var n int
	var utf8Text []byte
	var err error
	var buf [8192]byte
	c.conn, err = net.Dial("tcp", "ptt.cc:23")
	if err != nil {
		return utf8Text, errors.Wrap(err, "Connect fail")
	}
	err = c.ByPassRead()
	time.Sleep(1 * time.Second)
	n, err = c.conn.Read(buf[0:])

	utf8Text, _ = Big5toUTF8(buf[0:n])
	if strings.Contains(string(utf8Text), "系統過載") {
		return utf8Text, errors.New("系統過載")
	} else if strings.Contains(string(utf8Text), "請輸入代號") {
		_, err = c.Write(c.user, 1)
		if err != nil {
			return utf8Text, err
		}
		_, err = c.Write(c.pswd, 1)
		if err != nil {
			return utf8Text, err
		}
		n, err = c.conn.Read(buf[0:])
		utf8Text, err = Big5toUTF8(buf[0:n])
	}
	return utf8Text, nil
}
