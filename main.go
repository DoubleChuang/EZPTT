package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DoubleChuang/EZPTT/pttclient"

	"github.com/pkg/errors"
)

//Login 登入PTT
func Login(wg *sync.WaitGroup, user string, pswd string, outChan chan<- string, errChan chan<- error) {
	defer wg.Done()
	pttClient := pttclient.NewPTTClient(user, pswd)
	recv, err := pttClient.Login()
	if err == nil {
		str := string(recv)
		if strings.Contains(str, "密碼不對") {
			errChan <- errors.New(user + "密碼不對")
			return
		} else if strings.Contains(str, "您想刪除其他重複登入") {
			fmt.Println("刪除其他重複登入的連線....")
			pttClient.Write(pswd, 8)
			pttClient.ByPassRead()
		} else if strings.Contains(str, "請按任意鍵繼續") {
			pttClient.Write("", 2)
			pttClient.ByPassRead()
		} else if strings.Contains(str, "您要刪除以上錯誤嘗試") {
			pttClient.Write("y", 2)
			pttClient.ByPassRead()
		} else if strings.Contains(str, "您有一篇文章尚未完成") {
			pttClient.Write("q", 2)
			pttClient.ByPassRead()
		} else if strings.Contains(str, "登入中，請稍候...") {
			time.Sleep(2 * time.Second)
			pttClient.ByPassRead()
		} else {
			fmt.Println(str)
			errChan <- errors.New(user + "解析錯誤")
			return
		}
	} else {
		fmt.Println(err)
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

			go Login(&wg, row[0], row[1], outChan, errChan)
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
