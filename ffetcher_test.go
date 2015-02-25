package ffetcher

import (
	"fmt"
	"testing"
)

var fetcher Ffetcher = make(Ffetcher)

func TestCrawl(t *testing.T) {
	Crawl("http://golang.org/", 2, fetcher)

	for u, _ := range fetcher {
		//<-fetcher[u].done

		fmt.Println("fetcher[", u, "] = {\n   body\n   urls = [")
		for _, s := range fetcher[u].Urls {
			fmt.Println("      ", s, ",")
		}
		fmt.Println("   ]\n}")
	}
}
