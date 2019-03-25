package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	netUrl "net/url"
	"os"
	"strings"
	"time"
)

func main() {
	configFile := argParse()
	config := parseConfig(configFile)

	ready, exit := make(chan struct{}), make(chan struct{})

	c := newReceiver(ready, exit, config.RequestNumber)
	sendRequest(ready, c, config)

	<-exit
}

type CountData struct {
	total         int           // 请求总数
	successNumber int           // 成功总数
	badNumber     int           // 失败总数
	maxTime       time.Duration // 最长成功响应时间
	minTime       time.Duration // 最短成功响应时间
	totalTime     time.Duration // 总成功响应时间
	meanTime      time.Duration // 平均成功响应时间
}

type Config struct {
	RequestNumber int                    `json:"request_number"`
	Workers       int                    `json:"workers"`
	Timeout       time.Duration          `json:"timeout"`
	Method        string                 `json:"method"`
	Url           string                 `json:"url"`
	Headers       map[string]interface{} `json:"headers"`
	Data          map[string]interface{} `json:"data"`
}

type ResponseData struct {
	isSuccess bool          // 请求是否成功
	timing    time.Duration // 响应时间
}

func newReceiver(ready, exit chan struct{}, requestNumber int) chan ResponseData {
	c := make(chan ResponseData, 10)

	go func() {
		count := CountData{minTime: time.Minute}

		<-ready // 接受开始信号

		start := time.Now()
		for x := range c {
			count.total++
			if x.isSuccess {
				count.successNumber++
				count.totalTime += x.timing
				if x.timing > count.maxTime {
					count.maxTime = x.timing
				}
				if x.timing < count.minTime {
					count.minTime = x.timing
				}

			} else {
				count.badNumber++
			}

			if count.total >= requestNumber {
				count.meanTime = count.totalTime / time.Duration(count.successNumber)
				fmt.Printf(
					`
Summary:
	请求总数: %v
	成功总数: %v
	失败总数: %v
	最长成功响应时间: %v
	最短成功响应时间: %v
	平均成功响应时间: %v
	程序运行时间: %v
`, count.total, count.successNumber, count.badNumber, count.maxTime, count.minTime, count.meanTime, time.Now().Sub(start))

				close(exit) // 发送结束信号
				return
			}
		}
	}()

	return c
}
func sendRequest(ready chan struct{}, c chan ResponseData, config *Config) {

	for i := 0; i < config.Workers; i++ {
		go func() {

			<-ready // 接收开始信号

			for {
				body := generateBody(config.Method, config.Headers, config.Data)
				request, err := http.NewRequest(config.Method, config.Url, *body)

				if err != nil {
					log.Fatalln(err)
				}

				setHeaders(request, config.Headers)
				client := &http.Client{Timeout: time.Second * config.Timeout}

				start := time.Now()
				resp, err := client.Do(request) //发送请求

				if err != nil {
					println(err.Error())
					continue
				}
				end := time.Now()
				delta := end.Sub(start)

				_ = resp.Body.Close() //一定要关闭resp.Data

				isSuccess := 200 <= resp.StatusCode && resp.StatusCode < 300

				c <- ResponseData{isSuccess, delta}
			}
		}()
	}

	close(ready) // 发送开始信号
	println("start...")
}

const (
	JsonType = "application/json"
	FromType = "application/x-www-form-urlencoded"
)

var MethodsOfUsingBody = []string{http.MethodPost, http.MethodPut, http.MethodPatch}

func generateBody(method string, headers, data map[string]interface{}) *io.Reader {
	var body io.Reader = nil
	if stringInSlice(method, MethodsOfUsingBody) && len(data) != 0 {
		v, _ := headers["Content-Type"]
		switch str, _ := v.(string); str {
		case JsonType:
			body = dataToJson(data)
		case FromType:
			body = dataToFrom(data)
		default:
			log.Fatalln(fmt.Sprintf("%v don't support %v", method, v))
		}
	}
	return &body
}

func setHeaders(request *http.Request, headers map[string]interface{}) {
	for k, v := range headers {
		request.Header.Set(k, v.(string))
	}
}

func dataToJson(data map[string]interface{}) *bytes.Buffer {
	jsonString, err := json.Marshal(data)
	if err != nil {
		log.Fatalln(err)
	}
	return bytes.NewBuffer(jsonString)
}

func dataToFrom(data map[string]interface{}) *strings.Reader {
	body := netUrl.Values{}
	for k, v := range data {
		body.Set(k, v.(string))
	}
	return strings.NewReader(body.Encode())
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func argParse() string {
	configFile := flag.String("c", "config.json", "set configuration `file`")
	flag.Parse()
	return *configFile
}

func parseConfig(configFile string) *Config {
	/* 解析配置 */

	jsonFile, err := os.Open(configFile)
	if err != nil {
		log.Fatalln(err)
	}
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)

	var config Config
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		log.Fatalln(err)
	}
	return &config
}
