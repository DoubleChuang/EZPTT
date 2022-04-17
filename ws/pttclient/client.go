package pttclient

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/DoubleChuang/EZPTT/ws/wsclinet"
)

type PttClientStatus int

const (
	NotSign PttClientStatus = iota
	LoggedIn
)

type PttClient struct {
	clint      *wsclinet.WsClient
	Username   string
	Password   string
	context    context.Context
	cancelFunc context.CancelFunc
	status     PttClientStatus
}

func (c *PttClient) Monitor() {
	defer func() {
		log.Println("[" + c.Username + "] Monitor shutdown")
	}()
	for {
		bMsg, err := c.clint.Read()
		if err != nil {
			log.Println("read:", err)
			return
		}

		msg := string(bMsg)

		if strings.Contains(msg, "系統過載") {
			log.Println("PTT sever is overload")
			c.Close()
			return
		} else if strings.Contains(msg, "請輸入代號") {
			log.Println("logging in [" + c.Username + "]...")
			// input username
			if err := c.clint.WriteBinary([]byte(c.Username)); err != nil {
				log.Println(err)
				c.Close()
				return
			}

			// input password
			if err := c.clint.WriteBinary([]byte(c.Password)); err != nil {
				log.Println(err)
				c.Close()
				return
			}

			bMsg, err := c.clint.Read()
			if err != nil {
				log.Println(err)
				c.Close()
				return
			}
			msg = string(bMsg)

			if !strings.Contains(msg, "密碼正確！ 開始登入系統...") {
				log.Println("Failed to login")
				c.Close()
				return
			}

			log.Println(msg)
			c.status = LoggedIn
		}
	}
}

func NewPTTClient(username string, password string) (*PttClient, error) {
	wsHeaders := http.Header{
		"Origin": {"https://term.ptt.cc"},
	}

	c, err := wsclinet.NewWsClient("wss://ws.ptt.cc/bbs", wsHeaders)
	if err != nil {
		return nil, err
	}

	if res, err := c.Conn(); err != nil {
		errMsg := fmt.Sprint("Failed to Conn:", err, " ", res.Status)
		return nil, errors.New(errMsg)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &PttClient{
		clint:      c,
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
	if err := c.clint.Close(); err != nil {
		return err
	}
	c.clint = nil
	return nil
}
