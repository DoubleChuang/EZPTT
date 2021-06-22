package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DoubleChuang/EZPTT/pkg/config"
	. "github.com/DoubleChuang/EZPTT/pkg/log"
	"github.com/DoubleChuang/EZPTT/pttclient"
	"github.com/spf13/viper"

	"github.com/pkg/errors"
	"github.com/robfig/cron"
)

//Login 登入PTT
func Login(wg *sync.WaitGroup, user string, pswd string, outChan chan<- string, errChan chan<- error) {
	defer wg.Done()
	pttClient := pttclient.NewPTTClient(user, pswd)
	recv, err := pttClient.Login()
	if err != nil {
		Logger.Error(err)
		errChan <- errors.New("Server no power")
		return
	}
	defer func() {
		Logger.Info("登出" + pttClient.Username())
		pttClient.Logout()
	}()

	str := string(recv)
	switch {
	case strings.Contains(str, "密碼不對"):
		errChan <- errors.New(user + "密碼不對")
		return
	case strings.Contains(str, "您想刪除其他重複登入"):
		fmt.Println("刪除其他重複登入的連線....")
		if err = pttClient.Write(pswd, 8); err != nil {
			errChan <- err
			return
		}
		pttClient.ByPassRead()
	case strings.Contains(str, "請按任意鍵繼續"):
		if err = pttClient.Write("", 2); err != nil {
			errChan <- err
			return
		}
		pttClient.ByPassRead()
	case strings.Contains(str, "您要刪除以上錯誤嘗試"):
		if err = pttClient.Write("y", 2); err != nil {
			errChan <- err
			return
		}
		pttClient.ByPassRead()
	case strings.Contains(str, "您有一篇文章尚未完成"):
		if err = pttClient.Write("q", 2); err != nil {
			errChan <- err
			return
		}
		pttClient.ByPassRead()
	case strings.Contains(str, "登入中，請稍候..."):
		time.Sleep(2 * time.Second)

		pttClient.ByPassRead()
	default:
		//fmt.Println(str)
		errChan <- errors.New(user + "解析錯誤")
		return
	}

	outChan <- user
}

var csvFilePath string

const usageString string = `Usage:
    %s [flags] 
    
    Download a video from URL.
    Example: %s -i ./PttConfig.csv
Flags:`

func ParseFlag() {
	flag.Usage = func() {
		Logger.Infof(usageString, os.Args[0], os.Args[0])
		flag.PrintDefaults()
	}
	currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		Logger.Fatal(err)
	}
	csvFilePath = filepath.Join(currentDir, "PttConfig.csv")

	flag.StringVar(&csvFilePath, "i", csvFilePath, "The csv file.")
	flag.Parse()
	Logger.Info("Args:", flag.Args())
}

type PTTAccount struct {
	Username string
	Password string
}

func GetPTTAccountFromCsv(csvFilePath string) ([]PTTAccount, error) {
	Logger.Info("GetPTTAccountFromCsv")
	PTTAccounts := make([]PTTAccount, 0)
	csvFile, err := os.Open(csvFilePath)
	if err != nil {
		return PTTAccounts, err
	}
	defer csvFile.Close()
	csvReader := csv.NewReader(csvFile)
	for {
		row, err := csvReader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			Logger.Error(err) // or handle it another way
			continue
		}
		name, pswd := row[0], row[1]
		PTTAccounts = append(PTTAccounts, PTTAccount{name, pswd})
	}
	return PTTAccounts, nil
}

func LoginAll(pttAccounts []PTTAccount) {
	outChan := make(chan string)
	errChan := make(chan error)
	finishChan := make(chan struct{})

	wg := sync.WaitGroup{}

	for _, ptt := range pttAccounts {
		Logger.Infof("正在登入 %s ... \n", ptt.Username)
		wg.Add(1)
		go Login(&wg, ptt.Username, ptt.Password, outChan, errChan)
	}
	go func() {
		wg.Wait()
		close(finishChan)
	}()
Loop:
	for {
		select {
		case val := <-outChan:
			Logger.Info("登入成功", val)
		case err := <-errChan:
			Logger.Fatal("登入錯誤", err)
		case <-finishChan:
			break Loop
		case <-time.After(10 * time.Second):
			break Loop
		}
	}
}

func SetupCron() {
	c := cron.New()
	pttAccounts, err := GetPTTAccountFromCsv(csvFilePath)
	if err != nil {
		Logger.Fatal(err)
	}
	c.AddFunc(viper.GetString("CRON.SPEC"), func() {
		Logger.Info("Run LoginAll...")
		LoginAll(pttAccounts)
	})

	c.Start()
}

func main() {
	config.SetDefaults()
	InitLog()
	ParseFlag()
	SetupCron()

	t1 := time.NewTimer(time.Second * 10)
	for {
		select {
		case <-t1.C:
			t1.Reset(time.Second * 10)
		}
	}
}
