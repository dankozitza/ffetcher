package ffetcher

import (
	"encoding/json"
	"fmt"
	"github.com/dankozitza/dkutils"
	"github.com/dankozitza/jobdist"
	"github.com/dankozitza/sconf"
	"github.com/dankozitza/stattrack"
	"github.com/nelsam/requests"
	"io"
	"net/http"
	"regexp"
	//"strings"
)

var (
	stat              = stattrack.New("package initialized")
	ffetcher_template = map[string]interface{}{
		"ffetch_url":   string(""),
		"ffetch_depth": int(0)}
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
	conf := sconf.Inst()

	var scratch string = f[url].body
	var ret error = nil

	newconf, err := dkutils.DeepTypePersuade(new(int), conf["ffetcher_urls_size"])
	if err != nil {
		stat.Warn("get_urls: conf setting ffetcher_urls_size could not be " +
			"converted to int, setting it to 5. error: " + err.Error())
		conf["ffetcher_urls_size"] = int(5)
	} else {
		conf["ffetcher_urls_size"] = newconf.(int)
	}

	r, err := regexp.Compile("(((ht|f)tp(s?))\\://)?(www.|[a-zA-Z].)[a-zA-Z0-9\\-\\.]+\\.(com|edu|gov|mil|net|org|biz|info|name|museum|us|ca|uk|onion)(\\:[0-9]+)*(/($|[a-zA-Z0-9\\.\\,\\;\\?\\'\\\\\\+&amp;%\\$#\\=~_\\-]+))*")

	if err != nil {
		ret = err
	}

	(*f[url]).Urls = r.FindAllString(scratch, conf["ffetcher_urls_size"].(int))

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

	// check that we got a 200
	if res.StatusCode != 200 {
		return "", nil, nil
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

	// can't indent with this?
	//enc := json.NewEncoder(w)

	conf := sconf.Inst()
	ffetcher_template["links"] = []interface{}{
		&map[string]interface{}{
			"href": conf["ffetcher_index"].(string),
			"rel":  "index"}}

	params, err := requests.New(r).Params()

	if err != nil { // send the template
		fakeworker := FfetchWorker(Ffetcher(nil))
		fakejob := jobdist.New(ffetcher_template, nil, fakeworker)

		reply := fakejob.New_Form()

		//err := enc.Encode(reply)

		r_map, err := json.MarshalIndent(reply, "", "   ")
		if err != nil {
			stat.PanicErr("could not marshal ffetcher", err)
		}
		fmt.Fprint(w, string(r_map))
		return
	}

	var f Ffetcher = make(Ffetcher)

	worker := FfetchWorker(Ffetcher(f))

	job := jobdist.New(ffetcher_template, params, worker)

	if !job.Satisfies_Template() { // send the template
		reply := job.New_Form()
		r_map, err := json.MarshalIndent(reply, "", "   ")
		if err != nil {
			stat.PanicErr("could not marshal ffetcher", err)
		}
		fmt.Fprint(w, string(r_map))

	} else { // create the new resource and redirect the client
		redir_loc := job.Create_Redirect()
		fmt.Fprint(w, "<html><body>You are being <a href=\"http://")
		fmt.Fprint(w, conf["address"].(string) + ":" + conf["port"].(string))
		fmt.Fprint(w, redir_loc + "\">.</body></html>\n")
	}
}

type FfetchWorker Ffetcher

func (fw FfetchWorker) Work(result *map[string]interface{}) error {

	res := *result

	res["response"] = fw

	Crawl(res["ffetch_url"].(string), res["ffetch_depth"].(int), Ffetcher(fw))

	return nil
}
