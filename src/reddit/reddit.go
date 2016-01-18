package reddit

import (
	"fmt"
	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"net/http"
	"regexp"
)

func Search(url string) (string, bool) {
	resp, err := http.Get("https://www.reddit.com/search?q=url%3A" + url + "&sort=relevance&t=all")
	if err != nil {
		return "", false
	}
	root, err := html.Parse(resp.Body)
	if err != nil {
		return "", false
	}

	matcher := func(n *html.Node) bool {
		return scrape.Attr(n, "class") == "search-title may-blank"
	}
	m_comments := func(n *html.Node) bool {
		if n == nil {
			return false
		}
		return scrape.Attr(n, "class") == "search-comments may-blank"
	}
	m_subreddit := func(n *html.Node) bool {
		if n == nil {
			return false
		}
		return scrape.Attr(n, "class") == "search-subreddit-link may-blank"
	}
	m_time := func(n *html.Node) bool {
		if n == nil {
			return false
		}
		return scrape.Attr(n, "datetime") != ""
	}

	post, err_ := scrape.Find(root, matcher)
	if post == nil {
		return "", false
	}
	if post.Parent == nil {
		return "", false
	}
	if post.Parent.Parent == nil {
		return "", false
	}
	main := post.Parent.Parent
	s_comments := "%error%"
	s_time := "%error%"
	s_subreddit := "%error%"
	title := scrape.Text(post)
	href := scrape.Attr(post, "href")

	comments, err_ := scrape.Find(main, m_comments)
	if err_ == true {
		s_comments = scrape.Text(comments)
	}
	time, err_ := scrape.Find(main, m_time)
	if err_ == true {
		s_time = scrape.Text(time)
	}
	subreddit, err_ := scrape.Find(main, m_subreddit)
	if err_ == true {
		s_subreddit = scrape.Text(subreddit)
	}

	re := regexp.MustCompile("comments/([[:alnum:]]+)/")
	match := re.FindStringSubmatch(href)
	s_url := "https://redd.it/" + match[1]
	s_final := fmt.Sprintf("[Reddit %s] %s (%s) - %s [%s]\n", s_subreddit, title, s_url, s_comments, s_time)
	return s_final, true
}
