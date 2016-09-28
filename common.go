package gb

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type StopWatch struct {
	start   time.Time
	Elapsed time.Duration
}

func (s *StopWatch) Start() {
	s.start = time.Now()
}
func (s *StopWatch) Stop() {
	s.Elapsed = time.Now().Sub(s.start)
}

var errorCount map[string]int
var cancelCnt int = 0

func ReportAll() {
	for key, value := range errorCount {
		fmt.Println("count:[", value, "]:", key)
	}
}

func TraceException(msg interface{}) {
	if errorCount == nil {
		errorCount = make(map[string]int, 100)
	}

	var str = fmt.Sprintf("%s", msg)
	var c = errorCount[str]
	errorCount[str] = errorCount[str] + 1

	if c > 1 {
		return
	}

	if strings.HasSuffix(str, "net/http: request canceled") {
		if cancelCnt > 0 {
			return
		} else {
			cancelCnt++
		}
	}
	switch {
	case Verbosity > 1:
		// print recovered error and stacktrace
		var buffer bytes.Buffer
		buffer.WriteString(fmt.Sprintf("errors: %s\n", msg))
		for skip := 1; ; skip++ {
			pc, file, line, ok := runtime.Caller(skip)
			if !ok {
				break
			}
			f := runtime.FuncForPC(pc)
			buffer.WriteString(fmt.Sprintf("\t%s:%d %s()\n", file, line, f.Name()))
		}
		buffer.WriteString("\n")
		fmt.Fprint(os.Stderr, buffer.String())
	case Verbosity > 0:
		// print recovered error only
		fmt.Fprintf(os.Stderr, "recover: %v\n", msg)
	}
}
