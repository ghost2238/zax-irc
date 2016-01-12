package main

import (
	"encoding/json"
	"fmt"
	irc "github.com/fluffle/goirc/client"
	"github.com/mvdan/xurls"
	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"net/http"
	"os"
	"regexp"
	"strings"
	//"time"
)

type Channel struct {
	Chan     string
	Password string
}

/*type Message struct {
	Timestamp time
	Text      string
}*/

type User struct {
	Nicknames []string
	Clients   []string
	//Messages  []Message
	Urls []string
}

type Reddit struct {
}

type Config struct {
	Admin      string
	Username   string
	Nickname   string
	Server     string
	ReportChan string
	Channels   []Channel
	Handlers   []string
	News       []string
}

func search_reddit(url string) (string, bool) {
	resp, err := http.Get("https://www.reddit.com/search?q=url%3A" + url + "&sort=relevance&t=all")
	if err != nil {
		return "", false
	}
	root, err := html.Parse(resp.Body)
	if err != nil {
		return "", false
	}
	// define a matcher
	matcher := func(n *html.Node) bool {
		return scrape.Attr(n, "class") == "search-title may-blank"
	}
	m_comments := func(n *html.Node) bool { return scrape.Attr(n, "class") == "search-comments may-blank" }
	m_subreddit := func(n *html.Node) bool { return scrape.Attr(n, "class") == "search-subreddit-link may-blank" }
	m_time := func(n *html.Node) bool { return scrape.Attr(n, "datetime") != "" }

	post, err_ := scrape.Find(root, matcher)
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

func main() {
	fmt.Println("Loading config...")

	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	config := Config{}
	err := decoder.Decode(&config)
	if err != nil {
		fmt.Println("error:", err)
	}

	fmt.Println("Config loaded.")

	cfg := irc.NewConfig(config.Nickname)
	cfg.SSL = false
	cfg.Server = config.Server
	cfg.NewNick = func(n string) string { return n + "^" }
	c := irc.Client(cfg)

	c.HandleFunc(irc.CONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			for i := 0; i < len(config.Channels); i++ {
				ch := config.Channels[i]
				c.Join(ch.Chan, ch.Password)
			}
		})

	quit := make(chan bool)
	c.HandleFunc(irc.DISCONNECTED,
		func(conn *irc.Conn, line *irc.Line) { quit <- true })

	/*c.HandleFunc(irc.PING,
	func(conn *irc.Conn, line *irc.Line) {

	})*/

	c.HandleFunc(irc.PRIVMSG,
		func(conn *irc.Conn, line *irc.Line) {

			p := strings.Split(line.Src, "!")
			sender := p[0]
			text := line.Text()

			fmt.Println("[" + line.Target() + "] " + sender + ": " + text)

			if len(config.ReportChan) > 0 {
				c.Privmsg(config.ReportChan, "["+line.Target()+"] "+sender+": "+text)
			}

			if sender == config.Admin {
				if line.Text() == "die" {
					c.Quit("As you say.")
				}
			}
			if !(sender == "Wipe" && (strings.Contains(text, "Steam") || strings.Contains(text, "YouTube"))) {
				urls := xurls.Relaxed.FindAllString(text, -1)
				for i := 0; i < len(urls); i++ {
					reddit, success := search_reddit(urls[i])
					if success {
						c.Privmsg(line.Target(), reddit)
					} else {
						fmt.Println("Failed to retrieve reddit URL.")
					}
				}
			}

		})

	if err := c.Connect(); err != nil {
		fmt.Printf("Connection error: %s\n", err.Error())
	}

	<-quit
}
