package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	irc "github.com/fluffle/goirc/client"
	"github.com/mvdan/xurls"
	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"io/ioutil"
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

type SteamApp struct {
	Id           int
	AppType      string
	Name         string
	Developer    string
	Publisher    string
	ReleaseDate  string
	ReleaseYear  string
	Rating       float32
	Reviews      int
	InGame       int
	Achievements bool
	Linux        bool
	Windows      bool
	OSX          bool
	SinglePlayer bool
	MultiPlayer  bool
	MMO          bool
	VAC          bool
	EarlyAccess  bool
	Coop         bool
	Workshop     bool
	TradingCards bool
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
var file_history_writer *bufio.Writer

func history_escape(text string) string {
	return strings.Replace(text, ",", "\\,", -1)
}

func history_unescape(text string) string {
	return strings.Replace(text, "\\,", ",", -1)
}

func history_event(user, event, data string) {
	line := []string{"event", strconv.FormatInt(time.Now().Unix(), 10), user, history_escape(event), history_escape(data)}
	file_history_writer.WriteString(strings.Join(line, ",") + "\n")
	file_history_writer.Flush()
	s_data := Event{event, user, data, time.Now()}
	userdata[user].Events = append(userdata[user].Events, s_data)
	eventdata = append(eventdata, s_data)
}

func history_url(user, url string) {
	line := []string{"url", strconv.FormatInt(time.Now().Unix(), 10), user, history_escape(url)}
	file_history_writer.WriteString(strings.Join(line, ",") + "\n")
	file_history_writer.Flush()
	s_url := Url{url, time.Now()}
	userdata[user].Urls = append(userdata[user].Urls, s_url)
	urldata = append(urldata, s_url)
}

func history_message(user, channel, msg string) {
	line := []string{"msg", strconv.FormatInt(time.Now().Unix(), 10), user, channel, history_escape(msg)}
	file_history_writer.WriteString(strings.Join(line, ",") + "\n")
	file_history_writer.Flush()
	s_msg := Message{msg, user, channel, time.Now()}
	userdata[user].Messages = append(userdata[user].Messages, s_msg)
	msgdata = append(msgdata, s_msg)
}

func (app SteamApp) Features(sep string) string {
	features := []string{}
	if app.MultiPlayer {
		features = append(features, "MP")
	}
	if app.SinglePlayer {
		features = append(features, "SP")
	}
	if app.MMO {
		features = append(features, "MMO")
	}
	if app.Coop {
		features = append(features, "CO")
	}
	if app.VAC {
		features = append(features, "VAC")
	}
	if app.EarlyAccess {
		features = append(features, "EA")
	}
	if app.TradingCards {
		features = append(features, "TC")
	}
	if app.Achievements {
		features = append(features, "Ach")
	}
	if app.Workshop {
		features = append(features, "WS")
	}
	return strings.Join(features, sep)
}

func (app SteamApp) OS(sep string) string {
	os := []string{}
	if app.Windows {
		os = append(os, "Win")
	}
	if app.Linux {
		os = append(os, "Lin")
	}
	if app.OSX {
		os = append(os, "OSX")
	}

	return strings.Join(os, sep)
}

func steam_find_app(url string, index int) (int, bool) {
	resp, err := http.Get(url)
	if err != nil {
		return -1, false
	}
	root, err := html.Parse(resp.Body)
	if err != nil {
		return -1, false
	}

	matcher := func(n *html.Node) bool {
		return scrape.Attr(n, "class") == "responsive_search_name_combined"
	}

	games := scrape.FindAll(root, matcher)
	if games == nil || len(games) == 0 {
		return -1, false
	}
	// Last game
	if index == -1 {
		index = len(games) - 1
	} else if index == -2 {
		index = rand_int(0, len(games)-1)
	}

	game := games[index]
	href := game.Parent
	appid := 0
	re := regexp.MustCompile("/app/(\\d+?)/")
	fmt.Println(scrape.Attr(href, "href"))
	app_s := re.FindStringSubmatch(scrape.Attr(href, "href"))
	if app_s != nil {
		appid, _ = strconv.Atoi(app_s[1])
		fmt.Println("appid %d", appid)
	} else {
		return -1, false
	}
	return appid, true
}

func steam_get_appinfo(appid int) (SteamApp, bool) {

	s_appid := strconv.Itoa(appid)
	app := SteamApp{}

	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://steamdb.info/app/"+s_appid+"/info/", nil)
	if err != nil {
		fmt.Println(err.Error())
		return app, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; WOW64; rv:43.0) Gecko/20100101 Firefox/43.0")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error())
		return app, false
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err.Error())
		return app, false
	}
	s_body := string(body)
	s_body = strings.Replace(s_body, "\n", "", -1)
	//log.Println(s_body)
	re := regexp.MustCompile("<table class=\"table table-bordered table-hover table-dark\">(.+?)</table>")
	match := re.FindStringSubmatch(s_body)
	if match == nil {
		fmt.Println("Unable to find table.")
		return app, false
	} else {
		//fmt.Println(match[1])
	}
	table := match[1]

	re_releasedate := regexp.MustCompile("Release Date</td><td>(.+?)<i")
	re_inner := regexp.MustCompile("<.*?>(.+?)<")
	re = regexp.MustCompile("<td.*?>(.+?)</td>")
	matches := re.FindAllStringSubmatch(table, -1)
	release := re_releasedate.FindStringSubmatch(s_body)

	app.Id = appid
	app.AppType = matches[3][1]

	date := strings.Replace(release[1], ",", "", -1)
	date_p := strings.Split(date, " ")

	app.ReleaseDate = release[1]
	app.ReleaseYear = date_p[2]
	app.Name = html.UnescapeString(matches[5][1])
	app.Developer = html.UnescapeString(re_inner.FindStringSubmatch(matches[7][1])[1])
	app.Publisher = html.UnescapeString(re_inner.FindStringSubmatch(matches[9][1])[1])
	app.Linux = strings.Contains(table, "icon-linux")
	app.Windows = strings.Contains(table, "icon-windows")
	app.OSX = strings.Contains(table, "icon-osx")

	app.SinglePlayer = strings.Contains(s_body, "aria-label=\"Single-player\"")
	app.MultiPlayer = strings.Contains(s_body, "aria-label=\"Multi-player\"")
	app.Coop = strings.Contains(s_body, "aria-label=\"Co-op\"")
	app.MMO = strings.Contains(s_body, "aria-label=\"MMO\"")
	app.VAC = strings.Contains(s_body, "aria-label=\"Valve Anti-Cheat enabled\"")
	app.EarlyAccess = strings.Contains(s_body, "aria-label=\"Early Access\"")
	app.TradingCards = strings.Contains(s_body, "aria-label=\"Steam Trading Cards\"")
	app.Achievements = strings.Contains(s_body, "aria-label=\"Steam Achievements\"")

	app.Workshop = strings.Contains(s_body, "aria-label=\"Steam Workshop\"")

	return app, true
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

	reader := bufio.NewScanner(file_history)
	file_history_writer = bufio.NewWriter(file_history)

	for reader.Scan() {
		fmt.Println(reader.Text()) // Println will add back the final '\n'
	}

	fmt.Println("Parsing history...")
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
				if args[1] == "last" || args[1] == "l" {
					url = urls[len(urls)-1]
				}
				if args[1] == "random" || args[1] == "r" {
					url = urls[rand_int(0, len(urls)-1)]
				}
				if args[1] == "find" || args[1] == "f" {
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
				if args[1] == "last" || args[1] == "l" {
					msg = msgs[len(msgs)-2]
				}
				if args[1] == "random" || args[1] == "r" {
					msg = msgs[rand_int(0, len(msgs)-1)]
				}
				if args[1] == "find" || args[1] == "f" {
					expr := ""
					for i := 2; i < len(args); i++ {
						expr += expr + args[i]
					}
					re := regexp.MustCompile(expr)
					for _, i_msg := range msgs {
						match := re.FindStringSubmatch(i_msg.Msg)
						if match != nil {
							if !(strings.Contains(i_msg.Msg, fmt.Sprintf("%s %s", args[0], args[1]))) {
								msg = i_msg
							}
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
				steam_appid := 0

				if subcommand == "latest" || subcommand == "l" {
					steam_appid, success = steam_find_app("http://store.steampowered.com/search/?sort_by=Released_DESC&tags=-1&category1=998&page=1", 0)
				}
				if subcommand == "random" || subcommand == "r" {
					page := strconv.Itoa(rand_int(1, 286))
					steam_appid, success = steam_find_app("http://store.steampowered.com/search/?sort_by=Released_DESC&tags=-1&category1=998&page="+page, 0)
				}
				if subcommand == "find" || subcommand == "f" {
					re := regexp.MustCompile(fmt.Sprintf("%s %s ([[:alnum:]'*!_ ]+)", args[0], args[1]))
					match := re.FindStringSubmatch(text)
					if match == nil || len(match) == 0 {
						fmt.Println("Doesn't match.")
						return
					}
					fmt.Println("matched term: " + match[1])
					search_url := "http://store.steampowered.com/search/?snr=&term=" + match[1]
					fmt.Println("Search URL: " + search_url)
					steam_appid, success = steam_find_app(search_url, 0)
				}
				if success {
					fmt.Println("Found appid %d, retrieving info...", steam_appid)
					app, success2 := steam_get_appinfo(steam_appid)
					if success2 {
						info := fmt.Sprintf("[https://steamdb.info/app/%d/] \"%s\" (%s by %s) %s - [%s]", app.Id, app.Name, app.ReleaseYear, app.Developer, app.OS("/"), app.Features("/"))
						c.Privmsg(line.Target(), info)
						fmt.Println(info)
					} else {
						fmt.Println("Failed to retrieve steamapp info.")
					}

				} else {
					fmt.Println("Failed to retrieve appid from search.")
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
	time.Sleep(1000 * time.Millisecond)
}
