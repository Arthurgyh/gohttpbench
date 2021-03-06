package gb

import (
	"bytes"
	"crypto/tls"
	//	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	//	"runtime/pprof"
	"github.com/go-errors/errors"

	"github.com/tsenart/tb"
)

var (
	freq   time.Duration
	bucket *tb.Bucket
)

func SetRatelimit(rate int64) *tb.Bucket {
	fmt.Println("SetRatelimit: --- : ", rate)
	freq = time.Duration(1e9 / rate)
	bucket = tb.NewBucket(rate, freq)
	return bucket
}

func TakeRatelimitToken(i int) {
	if bucket != nil {
		got := bucket.Take(1)
		for got != 1 {
			got = bucket.Take(1)
			time.Sleep(freq)
		}
		//var str = time.Now().Format("2006-01-02 15:04:05.000")
		//fmt.Printf("%02d %s %s\n", i, " >", str)
	}
}

const (
	FieldServerName  = "ServerName"
	FieldContentSize = "ContentSize"
	MaxBufferSize    = 8192
)

var (
	ErrInvalidContnetSize = errors.New("invalid content size")
)

type CustomRequest interface {
	Prepare(config *Config, request *http.Request, index int) (*http.Request, error)
	HandleResult(wk *HTTPWorker, response *http.Response) (n int64, err error)
}

type HTTPWorker struct {
	c         *Context
	client    *http.Client
	jobs      chan *http.Request
	collector chan *Record
	discard   io.ReaderFrom
	Custom    CustomRequest
}

func NewHTTPWorker(context *Context, jobs chan *http.Request, collector chan *Record) *HTTPWorker {

	var buf []byte
	contentSize := context.GetInt(FieldContentSize)
	if contentSize < MaxBufferSize {
		buf = make([]byte, contentSize)
	} else {
		buf = make([]byte, MaxBufferSize)
	}

	return &HTTPWorker{
		context,
		NewClient(context.config),
		jobs,
		collector,
		&Discard{buf},
		nil,
	}
}

const profila = "D:/dev/ops/ops_client_test/client_test/loadtest/bin/cpuprofile_a%02d.prof"

func (h *HTTPWorker) GetReader() io.ReaderFrom {
	return h.discard
}
func (h *HTTPWorker) Run(i int) {
	var fpath = fmt.Sprintf(profila, i)
	_ = fpath
	//	fmt.Println(fpath)

	h.c.start.Done()
	h.c.startRun.Wait()

	timer := time.NewTimer(h.c.config.executionTimeout)
	var count int = 0
	for job := range h.jobs {

		TakeRatelimitToken(i)
		count++
		timer.Reset(h.c.config.executionTimeout)
		asyncResult := h.send(job)

		select {
		case record := <-asyncResult:
			if !h.c.config.skipFirst || count > 1 {
				h.collector <- record
			}

		case <-timer.C:
			h.collector <- &Record{Error: &ResponseTimeoutError{errors.New("execution timeout")}}
			h.client.Transport.(*http.Transport).CancelRequest(job)

		case <-h.c.stop:
			h.client.Transport.(*http.Transport).CancelRequest(job)
			timer.Stop()
			return
		}
	}
	timer.Stop()
}

func (h *HTTPWorker) send(request *http.Request) (asyncResult chan *Record) {

	asyncResult = make(chan *Record, 1)
	go func() {
		record := &Record{}
		sw := &StopWatch{}
		sw.Start()

		var contentSize int64
		_ = contentSize

		defer func() {
			sw.Stop()
			record.responseTime = sw.Elapsed

			if r := recover(); r != nil {
				fmt.Printf("err recovered %s \n\n", errors.Wrap(r, 2).ErrorStack())

				if Err, ok := r.(error); ok {
					record.Error = Err

				} else {
					record.Error = &ExceptionError{errors.New(fmt.Sprint(r))}
				}

			} else {
				record.contentSize = contentSize

			}

			if record.Error != nil {
				TraceException(record.Error.Error())
			}

			asyncResult <- record
		}()

		var retry = 3
		var (
			resp *http.Response
			err  error
		)
		for {
			resp, err = h.client.Do(request)
			retry = retry - 1
			if err != nil {
				if retry <= 0 {
					time.Sleep(time.Second)
					record.Error = &ConnectError{err}
					fmt.Println(" ......stop retry...." + err.Error())
					return
				} else {
					fmt.Println(" ......retry...." + err.Error())
				}
			} else {
				break
			}
		}

		defer resp.Body.Close()

		if h.Custom != nil {
			contentSize, err = h.Custom.HandleResult(h, resp)
		} else {
			contentSize, err = h.discard.ReadFrom(resp.Body)
		}

		if resp.StatusCode < 200 || resp.StatusCode > 300 {
			record.Error = &ResponseError{errors.Errorf("Response is %d", resp.StatusCode)}
			//record.Error = &ResponseError{err}
			//return
		} else if err != nil {
			fmt.Printf("err in read: %s\n", err.Error())
			if err == io.ErrUnexpectedEOF {
				record.Error = &LengthError{ErrInvalidContnetSize}
				//return
			}

			record.Error = &ReceiveError{err}
			//return
		}

	}()
	return asyncResult
}

type Discard struct {
	blackHole []byte
}

func (d *Discard) ReadFrom(r io.Reader) (n int64, err error) {
	readSize := 0
	for {
		readSize, err = r.Read(d.blackHole)
		n += int64(readSize)
		if err != nil {
			if err == io.EOF {
				return n, nil
			}
			return
		}
	}
}
func (d *Discard) GetBuffer() []byte {
	return d.blackHole
}

func DetectHost(context *Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("err recovered %s \n\n", errors.Wrap(r, 2).ErrorStack())
			TraceException(r)
		}
	}()

	client := NewClient(context.config)
	reqeust, err := NewHTTPRequest(context.config)
	if err != nil {
		return
	}

	resp, err := client.Do(reqeust)

	if err != nil {
		return
	}

	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	context.SetString(FieldServerName, resp.Header.Get("Server"))
	headerContentSize := resp.Header.Get("Content-Length")

	if headerContentSize != "" {
		contentSize, _ := strconv.Atoi(headerContentSize)
		context.SetInt(FieldContentSize, contentSize)
	} else {
		context.SetInt(FieldContentSize, len(body))
	}

	return
}

func NewClient(config *Config) *http.Client {

	// skip certification check for self-signed certificates
	tlsconfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// TODO: tcp options
	// TODO: monitor tcp metrics
	transport := &http.Transport{
		DisableCompression:  !config.gzip,
		DisableKeepAlives:   !config.keepAlive,
		TLSClientConfig:     tlsconfig,
		MaxIdleConnsPerHost: config.concurrency * 2,
	}

	if config.proxyURL != nil {
		transport.Proxy = http.ProxyURL(config.proxyURL)
	}

	return &http.Client{Transport: transport}
}
func simpleNewHTTPRequest(config *Config) (request *http.Request, err error) {
	return nil, nil
}
func NewHTTPRequest(config *Config) (request *http.Request, err error) {

	var body io.Reader

	if config.method == "POST" || config.method == "PUT" {
		body = bytes.NewReader(config.bodyContent)
	}

	request, err = http.NewRequest(config.method, config.url, body)

	if err != nil {
		return
	}

	fmt.Println("Content-Type", config.contentType)
	request.Header.Set("Content-Type", config.contentType)
	request.Header.Set("User-Agent", config.userAgent)

	if config.keepAlive {
		request.Header.Set("Connection", "keep-alive")
	}

	for _, header := range config.headers {
		pair := strings.Split(header, ":")
		request.Header.Add(pair[0], pair[1])
	}

	for _, cookie := range config.cookies {
		pair := strings.Split(cookie, "=")
		c := &http.Cookie{Name: pair[0], Value: pair[1]}
		request.AddCookie(c)
	}

	if config.basicAuthentication != "" {
		pair := strings.Split(config.basicAuthentication, ":")
		request.SetBasicAuth(pair[0], pair[1])
	}

	return
}

func simpleCopyHTTPRequest(config *Config, request *http.Request) *http.Request {
	newRequest := *request
	if request.Body != nil {
		newRequest.Body = ioutil.NopCloser(bytes.NewReader(config.bodyContent))
	}
	return &newRequest
}

func CopyHTTPRequest(config *Config, request *http.Request) *http.Request {

	return simpleCopyHTTPRequest(config, request)
}

type LengthError struct {
	err error
}

func (e *LengthError) Error() string {
	return e.err.Error()
}

type ConnectError struct {
	err error
}

func (e *ConnectError) Error() string {
	return e.err.Error()
}

type ReceiveError struct {
	err error
}

func (e *ReceiveError) Error() string {
	return e.err.Error()
}

type ExceptionError struct {
	err error
}

func (e *ExceptionError) Error() string {
	return e.err.Error()
}

type ResponseError struct {
	err error
}

func (e *ResponseError) Error() string {
	return e.err.Error()
}

type ResponseTimeoutError struct {
	err error
}

func (e *ResponseTimeoutError) Error() string {
	return e.err.Error()
}
