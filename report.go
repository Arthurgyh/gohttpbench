package gb

import (
	"bytes"
	"fmt"
	"math"
	"net/url"
	"sort"
	"time"
)

func PrintHeader() {
	fmt.Println(`
This is GoHttpBench, Version ` + GBVersion + `, https://github.com/parkghost/gohttpbench
Author: Brandon Chen, Email: parkghost@gmail.com
Licensed under the MIT license
`)
}

func div(dur time.Duration, count int) (rez float64) {
	if count == 0 {
		return 0
	} else {
		return float64(dur.Nanoseconds()) / float64(time.Millisecond) / float64(count)
	}
}

func PrintReport(context *Context, stats *Stats) {

	var buffer bytes.Buffer

	config := context.config
	responseTimeData := stats.responseTimeData
	totalFailedReqeusts := stats.totalFailedReqeusts
	totalRequests := stats.totalRequests
	totalExecutionTime := stats.totalExecutionTime
	totalResponseTime := stats.totalResponseTime
	totalReceived := stats.totalReceived

	URL, _ := url.Parse(config.url)

	fmt.Fprint(&buffer, "\n\n")
	fmt.Fprintf(&buffer, "Server Software:        %s\n", context.GetString(FieldServerName))
	fmt.Fprintf(&buffer, "Server Hostname:        %s\n", config.host)
	fmt.Fprintf(&buffer, "Server Port:            %d\n\n", config.port)

	fmt.Fprintf(&buffer, "Document Path:          %s\n", URL.RequestURI())
	fmt.Fprintf(&buffer, "Document Length:        %d bytes\n\n", context.GetInt(FieldContentSize))

	fmt.Fprintf(&buffer, "Concurrency Level:      %d\n", config.concurrency)
	fmt.Fprintf(&buffer, "Time taken for tests:   %.6f seconds\n", totalExecutionTime.Seconds())
	fmt.Fprintf(&buffer, "Time taken in millis:   %d ms\n", int64(totalResponseTime)/int64(time.Millisecond))
	fmt.Fprintf(&buffer, "Complete requests:      %d\n", totalRequests)
	fmt.Fprintf(&buffer, "Success requests:       %d\n", stats.totalSuccess)
	if totalFailedReqeusts == 0 {
		fmt.Fprintln(&buffer, "Failed requests:        0")
	} else {
		fmt.Fprintf(&buffer, "Failed requests:        %d\n", totalFailedReqeusts)

		fmt.Fprint(&buffer, "Failed types and avg Query Times(ms)\n")
		fmt.Fprint(&buffer, "       \tSuccess\tConnect\tReceive\tResponse Length\tExceptions\n")
		fmt.Fprintf(&buffer, "Total:\t%d \t%d \t%d \t%d \t %d \t%d\n",
			stats.totalSuccess, stats.errConnect, stats.errReceive, stats.errResponse, stats.errLength, stats.errException)
		fmt.Fprintf(&buffer, "Times:\t%.2f\t%.2f\t%.2f\t%.2f\t %.2f\t%.2f\n",
			div(stats.totalResponseTime, stats.totalSuccess),
			div(stats.errConnectDur, stats.errConnect),
			div(stats.errReceiveDur, stats.errReceive),
			div(stats.errResponseDur, stats.errResponse),
			div(stats.errLengthDur, stats.errLength),
			div(stats.errExceptionDur, stats.errException))

		fmt.Fprintf(&buffer, "   (Connect: %d, Receive: %d, Response: %d, Length: %d, Exceptions: %d)\n", stats.errConnect, stats.errReceive, stats.errResponse, stats.errLength, stats.errException)
	}
	if stats.errResponse > 0 {
		fmt.Fprintf(&buffer, "Non-2xx responses:      %d\n", stats.errResponse)
	}
	fmt.Fprintf(&buffer, "HTML transferred:       %d bytes\n", totalReceived)

	if len(responseTimeData) > 0 && totalResponseTime > 0 {
		//var datacount = len(responseTimeData)
		stdDevOfResponseTime := stdDev(responseTimeData) / 1000000
		sort.Sort(durationSlice(responseTimeData))

		meanOfResponseTime := int64(totalResponseTime) / int64(totalRequests-totalFailedReqeusts) / 1000000
		medianOfResponseTime := responseTimeData[len(responseTimeData)/2] / 1000000
		minResponseTime := responseTimeData[0] / 1000000
		maxResponseTime := responseTimeData[len(responseTimeData)-1] / 1000000

		fmt.Fprintf(&buffer, "Requests per second:    %.2f [#/sec] (mean)\n", float64(totalRequests)/totalExecutionTime.Seconds())
		fmt.Fprintf(&buffer, "Time per request:       %.3f [ms] (mean)\n", float64(config.concurrency)*float64(totalExecutionTime.Nanoseconds())/1000000/float64(totalRequests))
		fmt.Fprintf(&buffer, "Time per request:       %.3f [ms] (mean, across all concurrent requests)\n", float64(totalExecutionTime.Nanoseconds())/1000000/float64(totalRequests))
		fmt.Fprintf(&buffer, "HTML Transfer rate:     %.2f [Kbytes/sec] received\n\n", float64(totalReceived/1024)/totalExecutionTime.Seconds())

		fmt.Fprint(&buffer, "Connection Times (ms)\n")
		fmt.Fprint(&buffer, "              min\tmean[+/-sd]\t\tmedian\tmax\n")
		fmt.Fprintf(&buffer, "Total:        %d     \t%d   ±%.2f \t %d \t%d\n\n",
			minResponseTime,
			meanOfResponseTime,
			stdDevOfResponseTime,
			medianOfResponseTime,
			maxResponseTime)

		fmt.Fprintln(&buffer, "Percentage of the requests served within a certain time (ms)")

		percentages := []int{50, 66, 75, 80, 90, 95, 98, 99}

		for _, percentage := range percentages {
			fmt.Fprintf(&buffer, " %d%%\t %d\n", percentage, responseTimeData[percentage*len(responseTimeData)/100]/1000000)
		}
		fmt.Fprintf(&buffer, " %d%%\t %d (longest request)\n", 100, maxResponseTime)
	}
	fmt.Println(buffer.String())

	//	if len(responseTimeData) > 0 {
	//		fmt.Println("\n---all---\n")
	//		for i, date := range responseTimeData {
	//			fmt.Printf(" %5d: %6d\n", i, int64(date)/int64(time.Millisecond))
	//		}
	//	}
}

type durationSlice []time.Duration

func (s durationSlice) Len() int           { return len(s) }
func (s durationSlice) Less(i, j int) bool { return s[i] < s[j] }
func (s durationSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// StdDev calculate standard deviation
func stdDev(data []time.Duration) float64 {
	var sum int64
	for _, d := range data {
		sum += int64(d)
	}
	avg := float64(sum / int64(len(data)))

	sumOfSquares := 0.0
	for _, d := range data {

		sumOfSquares += math.Pow(float64(d)-avg, 2)
	}
	return math.Sqrt(sumOfSquares / float64(len(data)))

}
