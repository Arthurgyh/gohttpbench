package gb

import (
	"net/http"
	"time"
)

type Benchmark struct {
	c         *Context
	Collector chan *Record
}

type Record struct {
	responseTime time.Duration
	contentSize  int64
	Error        error
}

const (
	GBVersion           = "0.1.9"
	MaxExecutionTimeout = time.Duration(30) * time.Second
	MaxRequests         = 50000 // for timelimit
)

var (
	Verbosity       = 1
	GoMaxProcs      int
	ContinueOnError bool
)

func NewBenchmark(context *Context) *Benchmark {
	Collector := make(chan *Record, context.config.requests)
	return &Benchmark{context, Collector}
}

func (b *Benchmark) Run() {

	jobs := make(chan *http.Request, b.c.config.concurrency*GoMaxProcs)

	for i := 0; i < b.c.config.concurrency; i++ {
		go NewHTTPWorker(b.c, jobs, b.Collector).Run(i)
	}

	base, _ := NewHTTPRequest(b.c.config)
	for i := 0; i < b.c.config.requests; i++ {
		jobs <- CopyHTTPRequest(b.c.config, base)
	}
	close(jobs)

	<-b.c.stop
}

func (b *Benchmark) RunCustom(custom CustomRequest) {

	//	reqscount := b.c.config.requests + b.c.config.concurrency
	var reqscount int
	if b.c.config.skipFirst {
		reqscount = b.c.config.requests + b.c.config.concurrency
	} else {
		reqscount = b.c.config.requests
	}

	//	jobs := make(chan *http.Request, b.c.config.concurrency*GoMaxProcs)
	jobs := make(chan *http.Request, reqscount)

	for i := 0; i < b.c.config.concurrency; i++ {
		h := NewHTTPWorker(b.c, jobs, b.Collector)
		h.Custom = custom
		go h.Run(i)
	}

	var base *http.Request
	if custom != nil {
		base, _ = custom.Prepare(b.c.config, nil, 0)
	} else {
		base, _ = NewHTTPRequest(b.c.config)
	}

	for i := 0; i < reqscount; i++ {
		if custom != nil {
			rq, _ := custom.Prepare(b.c.config, base, i)
			jobs <- rq
		} else {
			jobs <- CopyHTTPRequest(b.c.config, base)
		}
	}
	close(jobs)
	b.c.start.Done()

	<-b.c.stop
}
