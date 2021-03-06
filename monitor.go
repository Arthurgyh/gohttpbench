package gb

import (
	"fmt"
	"os"
	"os/signal"
	_ "sync/atomic"
	"time"
)

type Monitor struct {
	c         *Context
	collector chan *Record
	Output    chan *Stats
}

type Stats struct {
	responseTimeData []time.Duration

	totalRequests       int
	totalSuccess        int
	totalExecutionTime  time.Duration
	totalResponseTime   time.Duration
	totalReceived       int64
	totalFailedReqeusts int

	errLength       int
	errLengthDur    time.Duration
	errConnect      int
	errConnectDur   time.Duration
	errReceive      int
	errReceiveDur   time.Duration
	errException    int
	errExceptionDur time.Duration
	errResponse     int
	errResponseDur  time.Duration
}

func NewMonitor(context *Context, collector chan *Record) *Monitor {
	return &Monitor{context, collector, make(chan *Stats)}
}

func (m *Monitor) Run() {

	// catch interrupt signal
	userInterrupt := make(chan os.Signal, 1)
	signal.Notify(userInterrupt, os.Interrupt)

	stats := &Stats{totalResponseTime: time.Duration(0)}
	stats.responseTimeData = make([]time.Duration, 0, m.c.config.requests)

	var timelimiter <-chan time.Time
	if m.c.config.timelimit > 0 {
		t := time.NewTimer(time.Duration(m.c.config.timelimit) * time.Second)
		defer t.Stop()
		timelimiter = t.C
	}

	// waiting for all of http workers to start
	m.c.start.Wait()

	fmt.Printf("Benchmarking %s (be patient)\n", m.c.config.host)
	m.c.startRun.Done()
	sw := &StopWatch{}
	sw.Start()

loop:
	for {
		select {
		case record := <-m.collector:

			updateStats(stats, record)

			if record.Error != nil && !ContinueOnError {
				break loop
			}

			if stats.totalRequests >= 10 && stats.totalRequests%(m.c.config.requests/10) == 0 {
				fmt.Printf("Completed %d requests\n", stats.totalRequests)
			}

			if stats.totalRequests == m.c.config.requests {
				fmt.Printf("Finished %d requests\n", stats.totalRequests)
				break loop
			}

		case <-timelimiter:
			break loop
		case <-userInterrupt:
			break loop
		}
	}

	sw.Stop()
	stats.totalExecutionTime = sw.Elapsed

	// shutdown benchmark and all of httpworkers to stop
	close(m.c.stop)
	signal.Stop(userInterrupt)
	m.Output <- stats
}

func updateStats(stats *Stats, record *Record) {
	stats.totalRequests++

	if record.Error != nil {
		stats.totalFailedReqeusts++

		switch record.Error.(type) {
		case *ConnectError:
			stats.errConnect++
			stats.errConnectDur += record.responseTime
		case *ExceptionError:
			stats.errException++
			stats.errExceptionDur += record.responseTime
		case *LengthError:
			stats.errLength++
			stats.errLengthDur += record.responseTime
		case *ReceiveError:
			stats.errReceive++
			stats.errReceiveDur += record.responseTime
		case *ResponseError:
			stats.errResponse++
			stats.errResponseDur += record.responseTime
		default:
			stats.errException++
			stats.errExceptionDur += record.responseTime
		}

	} else {
		//		stats.totalResponseTime = time.Duration(int64(stats.totalResponseTime) + int64(record.responseTime))
		stats.totalResponseTime += record.responseTime
		stats.totalReceived += record.contentSize
		stats.responseTimeData = append(stats.responseTimeData, record.responseTime)
		stats.totalSuccess++
	}

}
