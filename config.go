package gb

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/maximmartynov/flag"
)

type Config struct {
	requests         int
	concurrency      int
	timelimit        int
	executionTimeout time.Duration

	method              string
	bodyContent         []byte
	contentType         string
	headers             []string
	cookies             []string
	gzip                bool
	keepAlive           bool
	basicAuthentication string
	userAgent           string

	proxyURL *url.URL

	url  string
	host string
	port int
}

func (c *Config) SetProxy(proxyURL string) error {
	if proxyURL == "" {
		return nil
	}
	if pURL, err := url.Parse(proxyURL); err == nil {
		c.proxyURL = pURL
		return nil
	} else {
		return err
	}
}
func (c *Config) GetKeepAlive() bool {
	return c.keepAlive
}

func LoadConfig() (config *Config, err error) {

	var flagSet = flag.NewFlagSet("gb", flag.IgnoreError)

	// setup command-line flags
	flagSet.IntVar(&Verbosity, "v", 0, "How much troubleshooting info to print")
	flagSet.IntVar(&GoMaxProcs, "G", runtime.NumCPU(), "Number of CPU")
	flagSet.BoolVar(&ContinueOnError, "r", false, "Don't exit when errors")

	request := flagSet.Int("n", 1, "Number of requests to perform")
	concurrency := flagSet.Int("c", 1, "Number of multiple requests to make")
	timelimit := flagSet.Int("t", 0, "Seconds to max. wait for responses")

	postFile := flagSet.String("p", "", "File containing data to POST. Remember also to set -T")
	proxyFlag := flagSet.String("x", "", "http proxy")
	putFile := flagSet.String("u", "", "File containing data to PUT. Remember also to set -T")
	headMethod := flagSet.Bool("i", false, "Use HEAD instead of GET")
	contentType := flagSet.String("T", "text/plain", "Content-type header for POSTing, eg. 'application/x-www-form-urlencoded' Default is 'text/plain'")

	var headers, cookies stringSet
	flagSet.Var(&headers, "H", "Add Arbitrary header line, eg. 'Accept-Encoding: gzip' Inserted after all normal header lines. (repeatable)")
	flagSet.Var(&cookies, "C", "Add cookie, eg. 'Apache=1234. (repeatable)")

	basicAuthentication := flagSet.String("A", "", "Add Basic WWW Authentication, the attributes are a colon separated username and password.")
	keepAlive := flagSet.Bool("k", false, "Use HTTP KeepAlive feature")
	gzip := flagSet.Bool("z", false, "Use HTTP Gzip feature")

	showHelp := flagSet.Bool("h", false, "Display usage information (this message)")

	var defaultErrmsg string = ""
	flagSet.Usage = func() {
		if defaultErrmsg != "" {
			fmt.Printf("%s\n", defaultErrmsg)
		}
		fmt.Print("Usage: gb [options] http[s]://hostname[:port]/path\nOptions are:\n")
		flagSet.PrintDefaults()
	}

	flagSet.Parse(os.Args[1:])

	if *showHelp {
		flagSet.Usage()
		os.Exit(0)
	}

	if flagSet.NArg() != 1 {
		defaultErrmsg = "err:no url"
		flagSet.Usage()
		os.Exit(-1)
	}

	urlStr := strings.Trim(strings.Join(flagSet.Args(), ""), " ")
	isURL, _ := regexp.MatchString(`http.*?://.*`, urlStr)

	if !isURL {
		defaultErrmsg = "err:not url string"
		flagSet.Usage()
		os.Exit(-1)
	}

	// build configuration
	config = &Config{}
	config.requests = *request
	config.concurrency = *concurrency

	if err := config.SetProxy(*proxyFlag); err != nil {
		fmt.Println("proxy url is not well format.", err)
		flagSet.Usage()
		os.Exit(-1)
	}

	switch {
	case *postFile != "":
		config.method = "POST"
		if err = loadFile(config, *postFile); err != nil {
			return
		}
	case *putFile != "":
		config.method = "PUT"
		if err = loadFile(config, *putFile); err != nil {
			return
		}
	case *headMethod:
		config.method = "HEAD"
	default:
		config.method = "GET"
	}

	if *timelimit > 0 {
		config.timelimit = *timelimit
		if config.requests == 1 {
			config.requests = MaxRequests
		}
	}
	config.executionTimeout = MaxExecutionTimeout

	config.contentType = *contentType
	config.keepAlive = *keepAlive
	config.gzip = *gzip
	config.basicAuthentication = *basicAuthentication
	config.headers = []string(headers)
	config.cookies = []string(cookies)
	config.userAgent = "GoHttpBench/" + GBVersion

	URL, err := url.Parse(urlStr)
	if err != nil {
		return
	}
	config.host, config.port = extractHostAndPort(URL)
	config.url = urlStr

	if Verbosity > 1 {
		fmt.Printf("dump config: %#+v\n", config)
	}

	// validate configuration
	if config.requests < 1 || config.concurrency < 1 || config.timelimit < 0 || GoMaxProcs < 1 || Verbosity < 0 {
		err = errors.New("wrong number of arguments")
		return
	}

	if config.concurrency > config.requests {
		err = errors.New("Cannot use concurrency level greater than total number of requests")
		return
	}

	return

}

func loadFile(config *Config, filename string) error {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	config.bodyContent = bytes
	return nil
}

type stringSet []string

func (f *stringSet) String() string {
	return fmt.Sprint([]string(*f))
}

func (f *stringSet) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func extractHostAndPort(url *url.URL) (host string, port int) {

	hostname := url.Host
	pos := strings.LastIndex(hostname, ":")
	if pos > 0 {
		portInt64, _ := strconv.Atoi(hostname[pos+1:])
		host = hostname[0:pos]
		port = int(portInt64)
	} else {
		host = hostname
		if url.Scheme == "http" {
			port = 80
		} else if url.Scheme == "https" {
			port = 443
		} else {
			panic("unsupported protocol schema:" + url.Scheme)
		}
	}

	return
}
