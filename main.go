package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DoubleChuang/EZPTT/pttclient"
	"github.com/pkg/errors"
)

var (
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
	RED     = "\033[5;31;40m"
	GREEN   = "\033[0;32;40m"
	RESET   = "\033[0m"
)

func Init(
	infoHandle io.Writer,
	warningHandle io.Writer,
	errorHandle io.Writer) {

	Info = log.New(infoHandle,
		GREEN+"[INFO]   "+RESET,
		log.Ldate|log.Ltime /*|log.Lshortfile*/)

	Warning = log.New(warningHandle,
		RED+"[WARNING]"+RESET,
		log.Ldate|log.Ltime /*|log.Lshortfile*/)

	Error = log.New(errorHandle,
		RED+"[ERROR]  "+RESET,
		log.Ldate|log.Ltime /*|log.Lshortfile*/)
}

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
			if err = pttClient.Write(pswd, 8); err != nil {
				errChan <- err
				return
			}
			pttClient.ByPassRead()
		} else if strings.Contains(str, "請按任意鍵繼續") {
			if err = pttClient.Write("", 2); err != nil {
				errChan <- err
				return
			}
			pttClient.ByPassRead()
		} else if strings.Contains(str, "您要刪除以上錯誤嘗試") {
			if err = pttClient.Write("y", 2); err != nil {
				errChan <- err
				return
			}
			pttClient.ByPassRead()
		} else if strings.Contains(str, "您有一篇文章尚未完成") {
			if err = pttClient.Write("q", 2); err != nil {
				errChan <- err
				return
			}
			pttClient.ByPassRead()
		} else if strings.Contains(str, "登入中，請稍候...") {
			time.Sleep(2 * time.Second)
			pttClient.ByPassRead()
		} else {
			//fmt.Println(str)
			errChan <- errors.New(user + "解析錯誤")
			return
		}
	} else {
		fmt.Println(err)
		errChan <- errors.New("Server no power")
		return
	}
	outChan <- user
}

const usageString string = `Usage:
    %s [flags] 
    
    Download a video from URL.
    Example: %s -i ./PttConfig.csv
Flags:`

func main() {
	var csvFilePath string
	Init(os.Stdout, os.Stdout, os.Stderr)
	flag.Usage = func() {
		fmt.Printf(usageString, os.Args[0], os.Args[0])
		flag.PrintDefaults()
	}
	currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		Error.Fatal(err)
	}
	csvFilePath = filepath.Join(currentDir, "PttConfig.csv")

	flag.StringVar(&csvFilePath, "i", csvFilePath, "The csv file.")
	flag.Parse()
	Info.Println("Args:", flag.Args())

	outChan := make(chan string)
	errChan := make(chan error)
	finishChan := make(chan struct{})
	csvFile, err := os.Open(csvFilePath)
	if err != nil {
		Error.Fatal(err)
		return
	}
	defer csvFile.Close()

	csvReader := csv.NewReader(csvFile)
	wg := sync.WaitGroup{}
	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			Error.Fatal(err) // or handle it another way
		} else {
			wg.Add(1)
			Info.Printf("正在登入 %s ... \n",
				row[0])

			go Login(&wg, row[0], row[1], outChan, errChan)
		}
	}

	go func() {
		csvFile.Close()
		wg.Wait()
		close(finishChan)
	}()
Loop:
	for {
		select {
		case val := <-outChan:
			Info.Println("登入成功", val)
		case err := <-errChan:
			Error.Fatal("登入錯誤", err)
		case <-finishChan:
			break Loop
		case <-time.After(10 * time.Second):
			break Loop
		}
	}
}
