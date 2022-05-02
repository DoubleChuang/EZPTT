package pttclient

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/DoubleChuang/EZPTT/ws/wsclient"
)

type PttClientStatus int

const (
	NotSign PttClientStatus = iota
	LoggedIn
)

type PttClient struct {
	client     *wsclient.WsClient
	Username   string
	Password   string
	context    context.Context
	cancelFunc context.CancelFunc
	status     PttClientStatus
	mu         sync.Mutex
}

func (c *PttClient) InputUsername() error {
	if c.client == nil {
		return errors.New("client is close")
	}

	if err := c.client.WriteBinary([]byte(c.Username)); err != nil {
		log.Println("Failed to input username", err)
		return err
	}

	return nil
}

func (c *PttClient) InputPassword() error {
	if c.client == nil {
		return errors.New("client is close")
	}

	if err := c.client.WriteBinary([]byte(c.Password)); err != nil {
		log.Println("Failed to input username", err)
		return err
	}

	return nil
}

func (c *PttClient) InputAnyKey() error {
	// input any key to continue
	if err := c.client.WriteBinary([]byte("")); err != nil {
		log.Println("Failed to input any key", err)
		return err
	}
	return nil
}

func (c *PttClient) Monitor() {

	defer func() {
		log.Println("[" + c.Username + "] Monitor shutdown")
	}()
	if c.client == nil {
		return
	}

	defer func() {
		c.Close()
	}()

	for {
		if c.client == nil {
			return
		}
		bMsg, err := c.client.Read()
		if err != nil {
			log.Println("read:", err)
			return
		}

		msg := string(bMsg)

		if strings.Contains(msg, "系統過載") {
			log.Println("PTT sever is overload")
			return
		} else if strings.Contains(msg, "請輸入代號") {
			log.Println("logging in [" + c.Username + "]...")
			// input username
			if err := c.InputUsername(); err != nil {
				log.Println("Failed to input username", err)
				return
			}
			// input password
			if err := c.InputPassword(); err != nil {
				log.Println("Failed to input password", err)
				return
			}
			bMsg, err := c.client.Read()
			if err != nil {
				log.Println("Failed to read login screen", err)
				return
			}
			// read login screen
			msg = string(bMsg)
			time.Sleep(2 * time.Second)
			switch {
			case strings.Contains(msg, "登入中，請稍候..."):
				time.Sleep(2 * time.Second)
				log.Println("Login 1!!")
			case strings.Contains(msg, "密碼正確！ 開始登入系統..."):
				log.Println("Login 2!!")
			case strings.Contains(msg, "請按任意鍵繼續"):
				log.Println("Login 3!!")
				if err := c.InputAnyKey(); err != nil {
					log.Println("error:", err)
					return
				}
			}

			if err := c.InputAnyKey(); err != nil {
				log.Println("error:", err)
				return
			}

			c.mu.Lock()
			c.status = LoggedIn
			c.mu.Unlock()
		}
	}
}

func NewPTTClient(username string, password string) (*PttClient, error) {
	wsHeaders := http.Header{
		"Origin": {"https://term.ptt.cc"},
	}

	c, err := wsclient.NewWsClient("wss://ws.ptt.cc/bbs", wsHeaders)
	if err != nil {
		return nil, err
	}

	if res, err := c.Conn(); err != nil {
		errMsg := fmt.Sprint("Failed to Conn:", err, " ", res.Status)
		return nil, errors.New(errMsg)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &PttClient{
		client:     c,
		Username:   username,
		Password:   password,
		context:    ctx,
		cancelFunc: cancel,
	}

	go client.Monitor()

	return client, nil
}

func (c *PttClient) Login() error {
	ctx, _ := context.WithTimeout(c.context, 10*time.Second)

	for {
		select {
		case <-ctx.Done():
			return errors.New("login timeout")
		default:
			if c.status == LoggedIn {
				return nil
			}
			time.Sleep(1 * time.Second)
		}
	}

}

func (c *PttClient) Close() error {
	if c.client != nil {
		return nil
	}

	if err := c.client.Close(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.client = nil
	return nil
}
