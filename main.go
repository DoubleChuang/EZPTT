package main

import (
	"io"
	"os"
	"net"
	"fmt"
	"sync"
	"time"
	"bytes"
	"strings"
	"io/ioutil"
	"encoding/csv"
	"golang.org/x/text/transform"
	"golang.org/x/text/encoding/traditionalchinese"
)

func Big5toUTF8(s []byte) ([]byte, error) {
	reader := transform.NewReader(bytes.NewReader(s), traditionalchinese.Big5.NewDecoder())
	d, e := ioutil.ReadAll(reader)
	if e != nil {
		return nil, e
	}
	return d, nil
}

func Login(wg *sync.WaitGroup, host string, user string, pswd string) bool {
	var buf [8192]byte
	
	conn, err := net.Dial("tcp", host+":23")
	if err != nil {
		fmt.Sprint(os.Stderr, "Error: %s", err.Error())
		wg.Done()
		return false
	}

	n, err := conn.Read(buf[0:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		wg.Done()
		return false
	}

	time.Sleep(1 * time.Second)
	n, err = conn.Read(buf[0:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		wg.Done()
		return false
	}
	dd, _ := Big5toUTF8(buf[0:n])

	if strings.Contains(string(dd), "系統過載") {
		fmt.Println("系統過載")
		wg.Done()
		return false
	} else if strings.Contains(string(dd), "請輸入代號") {
		//fmt.Println("!!!!!!Username")
		n, err = conn.Write([]byte(user + "\r\n"))
		time.Sleep(1 * time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
			wg.Done()
			return false
		}
		n, err = conn.Write([]byte(pswd + "\r\n"))
		time.Sleep(1 * time.Second)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
			wg.Done()
			return false
		}

		n, err = conn.Read(buf[0:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
			wg.Done()
			return false
		}
		dd, _ = Big5toUTF8(buf[0:n])
		str := string(dd)
		if strings.Contains(str, "密碼不對") {
			fmt.Fprintln(os.Stderr, "密碼不對")
			wg.Done()
			return false
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
			fmt.Println(str)
			wg.Done()
			return false
		}
		fmt.Println(user, "登入成功")
	} else {
		fmt.Println("Server no power")
		wg.Done()
		return false
	}
	wg.Done()
	return true

}
func main() {
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
		}else{
			wg.Add(1)
			fmt.Println("正再登入", row[0], "...")
			go Login(&wg, "ptt.cc", row[0], row[1])			
		}
	}
	csvFile.Close()
	wg.Wait()
	return
}
