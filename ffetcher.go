package ffetcher

import (
	"encoding/json"
	"fmt"
	"github.com/dankozitza/jobdist"
	"github.com/dankozitza/stattrack"
	"github.com/nelsam/requests"
	"io"
	"net/http"
	"strings"
)

var (
	stat              = stattrack.New("package initialized")
	ffetcher_template = map[string]interface{}{
		"ffetch_url":   "",
		"ffetch_depth": 0}
)

type Fetcher interface {
	// Fetch returns the body of URL and
	// a slice of URLs found on that page.
	Fetch(url string) (body string, urls []string, err error)
}

// Crawl uses fetcher to recursively crawl
// pages starting with url, to a maximum of depth.
func Crawl(url string, depth int, fetcher Ffetcher) {
	// TODO: Fetch URLs in parallel.
	if depth <= 0 {
		return
	}
	_, urls, err := fetcher.Fetch(url)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, u := range urls {
		fmt.Printf("from: [%s], fetching: [%s]\n", url, u)
		Crawl(u, depth-1, fetcher)
	}

	return
}

type Ffetcher map[string]*Fresult

type Fresult struct {
	body string
	Urls []string
	done chan bool
}

func (f Ffetcher) get_urls(url string) error {
	largeurls := make([]string, 1024)
	var scratch string = f[url].body
	var ret error = nil

	var i int = 0
	for {
		if i >= len(largeurls) {
			ret = fmt.Errorf("get_urls: exceded length of urls array at %d", i)
			break
		}
		n := strings.Index(scratch, "href=\"")
		if n >= 0 {
			scratch = scratch[n+6:]
			en := strings.Index(scratch, "\"")
			if en >= 0 {

				if scratch[:en] == "" {
					scratch = scratch[en:]
					continue
				}

				sn := strings.Index(scratch[:en], "//")
				if sn == 0 {
					scratch = "http:" + scratch
					en = strings.Index(scratch, "\"")
				}

				hn := strings.Index(scratch[:en], "http:")
				if hn != 0 {
					hn = strings.Index(scratch[:en], "https:")
				}
				if hn != 0 {
					largeurls[i] = url + scratch[1:en]
					i++

				} else {
					largeurls[i] = scratch[:en]
					i++
				}
			}
			scratch = scratch[en:]

		} else {
			ret = nil
			break
		}
	}

	if i > 0 {
		(*f[url]).Urls = make([]string, i-1)
		(*f[url]).Urls = largeurls[:i-1]
		(*f[url]).done = make(chan bool)

	} else {
		(*f[url]).Urls = nil
	}

	//(*f[url]).done <- true

	return ret
}

func (f Ffetcher) Fetch(url string) (string, []string, error) {

	_, ok := f[url]
	if ok {
		return "", nil, fmt.Errorf("Fetch: %s: already fetched this url", url)
	}
	res, err := http.Get(url)
	if err != nil {
		return "", nil, fmt.Errorf("Fetch: %s: %s", url, err.Error())
	}
	var str_body string
	buff := make([]byte, 1024)
	for {
		n, err := res.Body.Read(buff)
		if err != nil && err != io.EOF {
			return "", nil, fmt.Errorf("Fetch: %s: %s", url, err.Error())
		}
		if n == 0 {
			break
		}

		str_body += string(buff[:n])
	}
	res.Body.Close()

	// make sure the body is not empty before setting f[url]
	if str_body != "" {
		f[url] = &Fresult{"", nil, nil}
		(*f[url]).body = str_body
		(*f[url]).done = make(chan bool)

	} else {
		return "", nil, fmt.Errorf("Fetch: %s: body was empty")
	}

	gu_err := f.get_urls(url)
	if gu_err != nil {
		fmt.Println(gu_err.Error())
	}
	return f[url].body, f[url].Urls, nil
}

type HTTPHandler Ffetcher

func (fhh HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	params, err := requests.New(r).Params()

	// if dkutils.DeepTypeCheck(conf["ffetcher_template"], params))

	if err != nil { // send the template
		//mbp, err := json.MarshalIndent(bodyParams, "", "   ")
		//fmt.Fprint(w, "stat id: ", stat.Id)
		fakeworker := FfetchWorker(Ffetcher(fhh))
		fakejob := jobdist.New(ffetcher_template, params, fakeworker)

		reply := fakejob.New_Form()
		r_map, err := json.MarshalIndent(reply, "", "   ")
		if err != nil {
			stat.PanicErr("could not marshal ffetcher", err)
		}
		fmt.Fprint(w, string(r_map))
		return

		//stat.PanicErr("could not get request params", err)

		//stat.PanicErr("could not marshal ffetcher", err)
	}

	fmt.Fprintln(w, "len(params) == ", len(params))

	for k, v := range params {
		fmt.Fprint(w, "params[", k, "] == ", v, "\n")
	}

	//params["message"] = "starting ffetcher job";

	worker := FfetchWorker(Ffetcher(fhh))

	job := jobdist.New(ffetcher_template, params, worker)

	if !job.Satisfies_Template() {
		reply := job.New_Form()
		r_map, err := json.MarshalIndent(reply, "", "   ")
		if err != nil {
			stat.PanicErr("could not marshal ffetcher", err)
		}
		fmt.Fprint(w, string(r_map))

	} else {
		redir_loc := job.Create_Redirect()
		fmt.Fprint(w, "<html><body>You are being <a href=\"http://localhost:9000"+redir_loc+"\">.</body></html>")
	}

	//m_map, err := json.MarshalIndent(params, "", "   ")
	//if err != nil {
	//	stat.PanicErr("could not marshal ffetcher", err)
	//}
	//fmt.Fprint(w, string(m_map))
}

type FfetchWorker Ffetcher

func (fw FfetchWorker) Work(result *map[string]interface{}) error {

	res := *result

	res["response"] = fw

	Crawl(res["ffetch_url"].(string), 4, Ffetcher(fw))

	return nil
}
