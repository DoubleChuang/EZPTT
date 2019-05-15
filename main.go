package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

func Big5toUTF8(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), traditionalchinese.Big5.NewDecoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func Login(wg *sync.WaitGroup, host string, user string, pswd string, outChan chan<- string, errChan chan<- error) {
	defer wg.Done()
	var buf [8192]byte

	conn, err := net.Dial("tcp", host+":23")
	if err != nil {
		errChan <- err
		return
	}

	n, err := conn.Read(buf[0:])
	if err != nil {
		errChan <- err
		return
	}

	time.Sleep(1 * time.Second)
	n, err = conn.Read(buf[0:])
	if err != nil {
		errChan <- err
		return
	}
	dd, _ := Big5toUTF8(buf[0:n])

	if strings.Contains(string(dd), "系統過載") {

		errChan <- errors.New("系統過載")
		return
	} else if strings.Contains(string(dd), "請輸入代號") {
		n, err = conn.Write([]byte(user + "\r\n"))
		time.Sleep(1 * time.Second)
		if err != nil {
			errChan <- err
			return
		}
		n, err = conn.Write([]byte(pswd + "\r\n"))
		time.Sleep(1 * time.Second)

		if err != nil {
			errChan <- err
			return
		}
		n, err = conn.Read(buf[0:])
		if err != nil {
			errChan <- err
			return
		}
		dd, _ = Big5toUTF8(buf[0:n])
		str := string(dd)
		if strings.Contains(str, "密碼不對") {
			errChan <- errors.New(user+"密碼不對")
			return
		} else if strings.Contains(str, "您想刪除其他重複登入") {
			fmt.Println("刪除其他重複登入的連線....")
			n, err = conn.Write([]byte(pswd + "\r\n"))
			time.Sleep(8 * time.Second)
			n, err = conn.Read(buf[0:])
		} else if strings.Contains(str, "請按任意鍵繼續") {
			n, err = conn.Write([]byte("\r\n"))
			time.Sleep(2 * time.Second)
			n, err = conn.Read(buf[0:])
		} else if strings.Contains(str, "您要刪除以上錯誤嘗試") {
			n, err = conn.Write([]byte("y\r\n"))
			time.Sleep(2 * time.Second)
			n, err = conn.Read(buf[0:])
		} else if strings.Contains(str, "您有一篇文章尚未完成") {
			n, err = conn.Write([]byte("q\r\n"))
			time.Sleep(2 * time.Second)
			n, err = conn.Read(buf[0:])
		} else {
			errChan <- errors.New(user+"解析錯誤")
			return
		}
	
	} else {
		errChan <- errors.New("Server no power")
		return
	}

	outChan <- user
	return

}
func main() {
	outChan := make(chan string)
	errChan := make(chan error)
	finishChan := make(chan struct{})
	csvFile, err := os.Open("./PttConfig.csv")
	if err != nil {
		panic(err)
	}
	defer csvFile.Close()

	csvReader := csv.NewReader(csvFile)
	wg := sync.WaitGroup{}
	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err) // or handle it another way
		} else {
			wg.Add(1)
			t := time.Now()
			fmt.Printf("[INFO] %d-%02d-%02dT%02d:%02d:%02d-00:00"+
				"正在登入 %s ... \n",
				t.Year(), t.Month(), t.Day(),
				t.Hour(), t.Minute(), t.Second(),
				row[0])

			go Login(&wg, "ptt.cc", row[0], row[1], outChan, errChan)
		}
	}

	go func() {
		csvFile.Close()
		wg.Wait()
		//fmt.Println("Finish all login")
		close(finishChan)
	}()
Loop:
	for {
		select {
		case val := <-outChan:
			fmt.Printf("\033[0;32;40m[INFO] %s登入成功\033[0m\n", val)
		case err := <-errChan:
			fmt.Printf("\033[5;31;40m[ERROR] %s\033[0m\n", err)
		case <-finishChan:
			break Loop
		case <-time.After(10 * time.Second):
			break Loop
		}
	}
	return
}
