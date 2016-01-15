package main

import (
	"encoding/json"
	"fmt"
	irc "github.com/fluffle/goirc/client"
	"github.com/mvdan/xurls"
	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Channel struct {
	Chan     string
	Password string
}

type Url struct {
	Url       string
	Timestamp time.Time
}

type Message struct {
	Msg       string
	User      string
	Channel   string
	Timestamp time.Time
}

type Event struct {
	Event     string
	User      string
	Data      string // e.g quit message
	Timestamp time.Time
}

type User struct {
	Messages []Message
	Events   []Event
	Urls     []Url
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

var msgdata []Message
var eventdata []Event
var urldata []Url
var userdata map[string]*User
var file_history *os.File

func history_escape(text string) string {
	return strings.Replace(text, ",", "\\,", -1)
}

func history_unescape(text string) string {
	return strings.Replace(text, "\\,", ",", -1)
}

func history_event(user, event, data string) {
	line := []string{"event", strconv.FormatInt(time.Now().Unix(), 10), user, history_escape(event), history_escape(data)}
	file_history.WriteString(strings.Join(line, ",") + "\n")
	file_history.Sync()
	s_data := Event{event, user, data, time.Now()}
	userdata[user].Events = append(userdata[user].Events, s_data)
	eventdata = append(eventdata, s_data)
}

func history_url(user, url string) {
	line := []string{"url", strconv.FormatInt(time.Now().Unix(), 10), user, history_escape(url)}
	file_history.WriteString(strings.Join(line, ",") + "\n")
	file_history.Sync()
	s_url := Url{url, time.Now()}
	userdata[user].Urls = append(userdata[user].Urls, s_url)
	urldata = append(urldata, s_url)
}

func history_message(user, channel, msg string) {
	line := []string{"msg", strconv.FormatInt(time.Now().Unix(), 10), user, channel, history_escape(msg)}
	file_history.WriteString(strings.Join(line, ",") + "\n")
	file_history.Sync()
	s_msg := Message{msg, user, channel, time.Now()}
	userdata[user].Messages = append(userdata[user].Messages, s_msg)
	msgdata = append(msgdata, s_msg)
}

func steam_search_request(url string) (string, bool) {
	resp, err := http.Get(url)
	if err != nil {
		return "", false
	}
	root, err := html.Parse(resp.Body)
	if err != nil {
		return "", false
	}

	matcher := func(n *html.Node) bool {
		return scrape.Attr(n, "class") == "responsive_search_name_combined"
	}

	games := scrape.FindAll(root, matcher)
	if games == nil || len(games) == 0 {
		return "", false
	}
	game := games[0]
	err_ := false
	title, err_title := scrape.Find(game, func(n *html.Node) bool { return scrape.Attr(n, "class") == "title" })
	href := game.Parent
	_, err_ = scrape.Find(game, func(n *html.Node) bool { return scrape.Attr(n, "class") == "platform_img win" })
	s_win := ""
	s_mac := ""
	s_linux := ""
	if err_ == true {
		s_win = "Win"
	}
	_, err_ = scrape.Find(game, func(n *html.Node) bool { return scrape.Attr(n, "class") == "platform_img mac" })
	if err_ == true {
		s_mac = "/Mac"
	}
	_, err_ = scrape.Find(game, func(n *html.Node) bool { return scrape.Attr(n, "class") == "platform_img linux" })
	if err_ == true {
		s_linux = "/Linux"
	}
	if err_title == true {
		return fmt.Sprintf("[Steam Release] %s - %s [%s%s%s]\n", scrape.Text(title), scrape.Attr(href, "href"), s_win, s_mac, s_linux), true
	}
	return "", false
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

func rand_int(min, max int) int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(max-min) + min
}

func is_command(text string, cmds []string) bool {
	for _, cmd := range cmds {
		if strings.HasPrefix(text, cmd) {
			return true
		}
	}
	return false
}

func get_quit_msg() string {

	switch rand_int(0, 4) {
	case 0:
		return "Uh, never mind."
	case 1:
		return "This system is too advanced for you."
	case 2:
		return "That was an illogical decision."
	case 3:
		return "Weeeeeeeeeeeeeeeeeeeeee[bzzt]"
	case 4:
		return "Didn't we have some fun, though?"
	}
	return "Interesting."
}

func main() {
	userdata = make(map[string]*User)
	fmt.Println("Loading config...")

	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	config := Config{}
	err := decoder.Decode(&config)
	if err != nil {
		fmt.Println("error:", err)
	}

	fmt.Println("Config loaded.")

	fmt.Println("Load history...")
	file_history, err = os.OpenFile("history.log", os.O_CREATE, os.ModeAppend)
	if err != nil {
		fmt.Println("Unable to open history.log")
		os.Exit(-1)
	}
	fmt.Println("History loaded.")

	cfg := irc.NewConfig(config.Nickname)
	cfg.SSL = false
	cfg.Server = config.Server
	cfg.NewNick = func(n string) string { return n + "^" }
	c := irc.Client(cfg)

	quit := make(chan bool)
	last_url := ""

	c.HandleFunc(irc.CONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			for i := 0; i < len(config.Channels); i++ {
				ch := config.Channels[i]
				c.Join(ch.Chan, ch.Password)
			}
		})

	c.HandleFunc(irc.DISCONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			fmt.Println("Disconnected")
			quit <- true
		})

	c.HandleFunc(irc.JOIN,
		func(conn *irc.Conn, line *irc.Line) {
			fmt.Println("[" + line.Target() + "] " + line.Nick + " (~" + line.Ident + "@" + line.Host + ") has joined.")
			history_event(line.Nick, "join", "")
			if line.Nick == config.Nickname {
				return
			}
			time.Sleep(100 * time.Millisecond)
		})

	c.HandleFunc(irc.QUIT,
		func(conn *irc.Conn, line *irc.Line) {
			fmt.Println("[" + line.Target() + "] " + line.Nick + " (" + line.Ident + "@" + line.Host + ") has quit.")
			history_event(line.Nick, "quit", line.Text())
			time.Sleep(100 * time.Millisecond)
		})

	c.HandleFunc(irc.PING,
		func(conn *irc.Conn, line *irc.Line) {
			fmt.Println("PING.")
			time.Sleep(100 * time.Millisecond)
		})

	c.HandleFunc(irc.PRIVMSG,
		func(conn *irc.Conn, line *irc.Line) {
			time.Sleep(100 * time.Millisecond)

			sender := line.Nick
			text := line.Text()
			channel := line.Target()
			fmt.Println("[" + line.Target() + "] " + sender + ": " + text)

			_, ok := userdata[sender]
			if !ok {
				m := []Message{}
				e := []Event{}
				u := []Url{}
				userdata[sender] = &User{m, e, u}
			}

			history_message(sender, line.Target(), text)

			if len(config.ReportChan) > 0 {
				c.Privmsg(config.ReportChan, "["+line.Target()+"] "+sender+": "+text)
			}

			if sender == config.Admin {
				if line.Text() == "!" || line.Text() == "<<" {
					c.Quit(get_quit_msg())
				}
			}

			cmd_rand := []string{".r", ".random"}
			cmd_steam := []string{".s", ".steam"}
			cmd_url := []string{".u", ".url"}
			cmd_msg := []string{".m", ".msg"}
			//cmd_seen := []string{".seen", ""}
			args := strings.Split(text, " ")
			user := ""

			// Parse user for history commands
			if is_command(text, cmd_url) || is_command(text, cmd_msg) {
				re := regexp.MustCompile(fmt.Sprintf("%s %s.+(u=[[:alnum:]]+)", args[0], args[1]))
				match := re.FindStringSubmatch(text)
				if match != nil {
					user = match[1]
				}
			}
			if is_command(text, cmd_url) {
				var url Url
				var urls []Url
				if user != "" {
					urls = userdata[user].Urls
				} else {
					urls = urldata
				}
				if args[1] == "last" {
					url = urls[len(urls)-1]
				}
				if args[1] == "find" {
					expr := ""
					for i := 2; i < len(args); i++ {
						expr += expr + args[i]
					}
					re := regexp.MustCompile(expr)
					for _, i_url := range urls {
						match := re.FindStringSubmatch(i_url.Url)
						if match != nil {
							url = i_url
						}
					}
				}
				if url.Url == "" {
					return
				}
				t := url.Timestamp
				c.Privmsg(channel, fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d] %v", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), url.Url))
				return
			}
			if is_command(text, cmd_msg) {
				var msg Message
				var msgs []Message
				if user != "" {
					msgs = userdata[user].Messages
				} else {
					msgs = msgdata
				}
				if args[1] == "last" {
					msg = msgs[len(msgs)-1]
				}
				if args[1] == "find" {
					expr := ""
					for i := 2; i < len(args); i++ {
						expr += expr + args[i]
					}
					re := regexp.MustCompile(expr)
					for _, i_msg := range msgs {
						match := re.FindStringSubmatch(i_msg.Msg)
						if match != nil {
							msg = i_msg
						}
					}
				}
				if msg.Msg == "" {
					return
				}
				t := msg.Timestamp
				c.Privmsg(channel, fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d] %v: %v", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), msg.User, msg.Msg))
				return
			}

			if is_command(text, cmd_rand) {
				if len(args) < 3 {
					return
				}
				min, err := strconv.ParseInt(args[1], 10, 32)
				if err != nil {
					fmt.Println("Failed to parse min.")
					return
				}
				max, err := strconv.ParseInt(args[2], 10, 32)
				if err != nil {
					fmt.Println("Failed to parse max.")
					return
				}

				c.Privmsg(line.Target(), "What about... "+strconv.Itoa(rand_int(int(min), int(max))))
			}

			if is_command(text, cmd_steam) {
				if len(args) < 2 {
					return
				}
				subcommand := args[1]
				success := false
				result := ""

				if subcommand == "latest" || subcommand == "l" {
					result, success = steam_search_request("http://store.steampowered.com/search/?sort_by=Released_DESC&tags=-1&category1=998&page=1")
				}
				if subcommand == "find" || subcommand == "f" {
					re := regexp.MustCompile(fmt.Sprintf("%s %s ([[:alnum:] ]+)", args[0], args[1]))
					match := re.FindStringSubmatch(text)
					if match == nil || len(match) == 0 {
						fmt.Println("Doesn't match.")
						return
					}
					search_url := "http://store.steampowered.com/search/?snr=&term=" + match[1]
					fmt.Println("Search URL: " + search_url)
					result, success = steam_search_request(search_url)
				}
				if success {
					c.Privmsg(line.Target(), result)
					fmt.Println(result)
				} else {
					fmt.Println("Failed to retrieve steam search result.")
				}
				return
			}
			// Handle URLs
			if !(sender == "Wipe" && (strings.Contains(text, "Steam") || strings.Contains(text, "YouTube"))) {
				urls := xurls.Relaxed.FindAllString(text, -1)
				for i := 0; i < len(urls); i++ {
					url := urls[i]
					fmt.Println("Found url " + url)
					history_url(sender, url)

					if url == last_url {
						fmt.Println("Matches same url as last time, ignore.")
						continue
					}
					reddit, success := search_reddit(url)
					if success {
						c.Privmsg(line.Target(), reddit)
					} else {
						fmt.Println("Failed to retrieve reddit URL for the link.")
					}
					last_url = url
				}
			}
			time.Sleep(100 * time.Millisecond)
		})

	if err := c.Connect(); err != nil {
		fmt.Printf("Connection error: %s\n", err.Error())
	}

	<-quit
	fmt.Println("Closing history.log")
	file_history.Close()
}
