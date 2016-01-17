package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"games"
	"github.com/mvdan/xurls"
	"github.com/sorcix/irc"
	"github.com/yhat/scrape"
	"golang.org/x/net/html"
	"math/rand"
	"net"
	"net/http"
	"os"
	"regexp"
	"steam"
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
	Channel   string
	Timestamp time.Time
}

type User struct {
	Messages []Message
	Events   []Event
	Urls     []Url
}

type OfflineMsg struct {
	Sender    string
	Recipient string
	Timestamp time.Time
}

type Config struct {
	Admin      string // nick:<expr> | host:<expr>
	Username   string
	Nickname   string
	Server     string
	ReportChan string
	UserAgent  string
	Channels   []Channel
	Handlers   []string
	News       []string
}

// History
var msgdata []Message
var eventdata []Event
var urldata []Url
var userdata map[string]*User
var file_history *os.File
var file_history_writer *bufio.Writer

var ignore_list []string // Ignore these usernames

var irc_writer *irc.Encoder
var config Config

var last_url string

// for context control.
var last_context_index int
var last_context_type string

func history_escape(text string) string {
	return strings.Replace(text, ",", "\\,", -1)
}

func history_unescape(text string) string {
	return strings.Replace(text, "\\,", ",", -1)
}

func history_event(user, event, data, channel string) {
	line := []string{"event", strconv.FormatInt(time.Now().Unix(), 10), user, channel, history_escape(event), data}
	file_history_writer.WriteString(strings.Join(line, ",") + "\n")
	file_history_writer.Flush()
	s_data := Event{event, user, data, channel, time.Now()}
	userdata[user].Events = append(userdata[user].Events, s_data)
	eventdata = append(eventdata, s_data)
}

func history_url(user, url string) {
	line := []string{"url", strconv.FormatInt(time.Now().Unix(), 10), user, url}
	file_history_writer.WriteString(strings.Join(line, ",") + "\n")
	file_history_writer.Flush()
	s_url := Url{url, time.Now()}
	userdata[user].Urls = append(userdata[user].Urls, s_url)
	urldata = append(urldata, s_url)
}

func history_message(user, channel, msg string) {
	line := []string{"msg", strconv.FormatInt(time.Now().Unix(), 10), user, channel, msg}
	file_history_writer.WriteString(strings.Join(line, ",") + "\n")
	file_history_writer.Flush()
	s_msg := Message{msg, user, channel, time.Now()}
	userdata[user].Messages = append(userdata[user].Messages, s_msg)
	msgdata = append(msgdata, s_msg)
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
		//fmt.Println("cmd: " + cmd)
		if strings.HasPrefix(text, cmd) {
			return true
		}
	}
	return false
}

func get_quit_msg() string {
	msg := []string{"Uh, never mind.", "This system is too advanced for you.", "That was an illogical decision.",
		"Weeeeeeeeeeeeeeeeeeeeee[bzzt]", "Didn't we have some fun, though?", "Your entire life has been a mathematical error."}
	return msg[rand_int(0, len(msg))]
}

/*func get_greeting_msg(user string) string {

}*/

func get_user_not_exists() string {
	msg := []string{"Who?", "Never heard of that human.", "Negative, no record found", "Did you spell that correctly?", "Are you sober?"}
	return msg[rand_int(0, len(msg))]
}

func get_insult() string {
	msg := []string{"What do you think you are doing?", "You're not a good person. You know that, right?", "Typical human.",
		"You don't even care. Do you?", "I guess we both know that isn't going to happen.", "All right, keep doing whatever it is you think you're doing.", "Are you sober?"}
	return msg[rand_int(0, len(msg))]
}

func main() {
	last_url = ""
	userdata = make(map[string]*User)
	fmt.Println("Loading config...")

	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	config = Config{}
	err := decoder.Decode(&config)
	if err != nil {
		fmt.Println("error:", err)
	}

	fmt.Println("Config loaded.")

	fmt.Println("Opening history...")
	time_history := time.Now()
	file_history, err = os.OpenFile("history.log", os.O_CREATE, os.ModeAppend)
	if err != nil {
		fmt.Println("Unable to open history.log")
		os.Exit(-1)
	}
	fmt.Println("Loading history...")
	reader := bufio.NewScanner(file_history)
	file_history_writer = bufio.NewWriter(file_history)
	for reader.Scan() {
		l := reader.Text()
		is_url := strings.HasPrefix(l, "url")
		is_msg := strings.HasPrefix(l, "msg")
		is_event := strings.HasPrefix(l, "event")
		parts := []string{}
		user := ""
		channel := ""
		timestamp := time.Time{}

		if is_url || is_msg || is_event {
			parts = strings.Split(l, ",")
			ts, _ := strconv.ParseInt(parts[1], 10, 64)
			timestamp = time.Unix(ts, 0)
			//fmt.Println(fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d]", timestamp.Year(), timestamp.Month(), timestamp.Day(), timestamp.Hour(), timestamp.Minute(), timestamp.Second()))
			user = parts[2]
			channel = parts[3]
			_, ok := userdata[user]
			if !ok {
				m := []Message{}
				e := []Event{}
				u := []Url{}
				userdata[user] = &User{m, e, u}
			}
		}
		if strings.HasPrefix(l, "event") {
			evt_s := parts[4]
			data := ""
			if len(parts) == 6 {
				data = parts[5]
			}

			event := Event{evt_s, user, data, channel, timestamp}
			userdata[user].Events = append(userdata[user].Events, event)
			eventdata = append(eventdata, event)
		}

		if strings.HasPrefix(l, "msg") {
			text := ""
			for i := 4; i < len(parts); i++ {
				text = text + parts[i]
			}
			msg := Message{text, user, channel, timestamp}
			userdata[user].Messages = append(userdata[user].Messages, msg)
			msgdata = append(msgdata, msg)
		}

		if strings.HasPrefix(l, "url") {
			url := Url{parts[3], timestamp}
			userdata[user].Urls = append(userdata[user].Urls, url)
			urldata = append(urldata, url)
		}
	}
	elapsed := time.Since(time_history)
	fmt.Printf("History loaded %d events, %d urls and %d messages in %f seconds.\n", len(eventdata), len(urldata), len(msgdata), elapsed.Seconds())
	fmt.Printf("Initializing IRC connection.")

	// Init IRC connection
	var conn net.Conn
	conn, err = net.Dial("tcp", config.Server)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("Connected.")
	fmt.Printf("Sending NICK/USER")
	messages := []*irc.Message{}
	messages = append(messages, &irc.Message{
		Command: irc.NICK,
		Params:  []string{config.Nickname},
	})
	messages = append(messages, &irc.Message{
		Command:  irc.USER,
		Params:   []string{config.Username, "0", "*"},
		Trailing: config.Nickname,
	})
	irc_reader := irc.NewDecoder(conn)
	irc_writer = irc.NewEncoder(conn)
	for _, msg := range messages {
		if err := irc_writer.Encode(msg); err != nil {
			fmt.Println(err)
		}
	}
	fmt.Printf("Start IRC process loop.")
	// IRC process loop
	for {
		conn.SetDeadline(time.Now().Add(300 * time.Second))
		msg, err := irc_reader.Decode()
		if err != nil {
			if err.Error() == "EOF" {
				fmt.Println("Disconnected.")
				break
			}
		}
		process_irc(msg, false)
	}
}

func process_irc(msg *irc.Message, is_debug bool) {
	last_url := ""
	var sendmsg *irc.Message
	fmt.Println(msg)
	pong := &irc.Message{
		Command:  irc.PONG,
		Params:   msg.Params,
		Trailing: msg.Trailing,
	}

	privmsg := &irc.Message{
		Command:  irc.PRIVMSG,
		Params:   []string{},
		Trailing: "",
	}

	quitmsg := &irc.Message{
		Command:  irc.QUIT,
		Params:   []string{},
		Trailing: get_quit_msg(),
	}

	if msg.Command == irc.PING {
		sendmsg = pong
		irc_writer.Encode(sendmsg)
		return
	}

	if msg.Command == irc.JOIN {
		fmt.Println("[" + msg.Trailing + "] " + msg.Name + " (" + msg.User + "@" + msg.Host + ") has joined.")
		//history_event(line.Nick, "join", line.Text(), "")
	}
	if msg.Command == irc.QUIT {
		fmt.Println("[" + msg.Trailing + "] " + msg.Name + " (" + msg.User + "@" + msg.Host + ") has quit.")
		//history_event(line.Nick, "quit", line.Text(), "")
	}

	if msg.Command == irc.RPL_ENDOFMOTD {
		if msg.IsServer() {
			for i := 0; i < len(config.Channels); i++ {
				ch := config.Channels[i]
				ch_param := []string{ch.Chan, ch.Password}
				join_msg := &irc.Message{
					Command: irc.JOIN,
					Params:  ch_param,
				}
				irc_writer.Encode(join_msg)
				fmt.Println(join_msg)
			}
		}
	}
	if msg.Command == irc.PRIVMSG {
		target := msg.Params[0]
		sender_host := msg.Host
		sender := msg.Name
		text := msg.Trailing

		// Remove control characters
		text = strings.Replace(text, "", "", -1)
		text = strings.Replace(text, "", "", -1)

		// reply to sender by default
		reply_ch := msg.Params[0]
		reply_msg := ""
		fmt.Println("[" + target + "] " + sender + ": " + text)

		/*if text == "VERSION" {
			reply_msg = "ZAX"
		}*/

		_, ok := userdata[sender]
		if !ok {
			m := []Message{}
			e := []Event{}
			u := []Url{}
			userdata[sender] = &User{m, e, u}
		}

		history_message(sender, target, text)
		if len(config.ReportChan) > 0 {
			reply_ch = config.ReportChan
			reply_msg = "[" + target + "] " + sender + ": " + text
		}

		cmd_admin := []string{"%%", "<<"}
		// Check if admin
		if is_command(text, cmd_admin) {
			re_adm := regexp.MustCompile("(nick|host):(.+)")
			criteria := re_adm.FindStringSubmatch(config.Admin)
			//is_admin := false
			match_str := ""

			if criteria == nil {
				fmt.Println("Unable to parse admin criteria.")
				return
			}
			re_adm_eval := regexp.MustCompile(criteria[2])
			if criteria[1] == "nick" {
				match_str = sender
			}
			if criteria[1] == "host" {
				match_str = sender_host
			}
			if !re_adm_eval.MatchString(match_str) {
				fmt.Println(fmt.Sprintf("Didn't pass the criteria: %s:%s", criteria[1], criteria[2]))
				return
			}
			fmt.Println(fmt.Sprintf("Passed the criteria: %s:%s with %s", criteria[1], criteria[2], match_str))
			if text == "<<" {
				sendmsg = quitmsg
			}
		}

		cmd_rand := []string{".r", ".random"}
		cmd_steam := []string{".s", ".steam"}
		cmd_game := []string{".g", ".game"}
		cmd_url := []string{".u", ".url"}
		cmd_msg := []string{".m", ".msg"}
		//cmd_youtube := []string{".y", ".youtube", ".yt"}
		//cmd_seen := []string{"!"}
		cmd_help := []string{"?h"}
		//cmd_seen := []string{".seen", ""}
		args := strings.Split(text, " ")
		if is_command(text, cmd_help) {
			if text == "?h" {
				reply_msg = "Cmds: [[.g(ame) .r(andom) .s(team) .u(rl) .m(sg) !]] -- Type ?h <cmd> for more info."
			}
			if args[1] == "!" {
				reply_msg = "Checks when user was last seen. Syntax: !<username>"
			}
			if is_command(args[1], cmd_rand) {
				reply_msg = "Generate random number. Syntax: .random <min> <max>"
			}
			if is_command(args[1], cmd_steam) {
				if len(args) == 3 {
					if args[2] == "symbols" {
						reply_msg = "MP=MultiPlayer, SP=SinglePlayer, CO=Co-op VAC=Valve Anti-Cheat, TC=Trading Card, Ach=Achievments, EA=Early Access, WS=Workshop support"
					}
				} else {
					reply_msg = "Search steam. For result symbols type '?h .s symbols' Syntax: .steam [ find | latest | random | trending | appid] <expression>"
				}
			}
			if is_command(args[1], cmd_game) {
				reply_msg = "Search for game info. Syntax: .game <query>"
			}
			if is_command(args[1], cmd_msg) {
				reply_msg = "Search message log. Syntax: .msg [ find | latest | random ] <expression>"
			}
			if is_command(args[1], cmd_url) {
				reply_msg = "Search URL log. Syntax: .url [ find | latest | random ] <expression>"
			}
		}
		/*if is_command(text, cmd_seen) {
			fmt.Println("Executing seen command.")
			seen_user := strings.Replace(args[0], "!", "", -1)
			if seen_user == "" {
				return
			}
			if seen_user == sender {
				reply_msg = get_insult()
			}

			state := c.StateTracker().GetNick(seen_user)
			if state != nil {
				user_channels := state.Channels
				for i := 0; i < len(config.Channels); i++ {
					ch := config.Channels[i]
					_, exists := user_channels[ch.Chan]
					if exists {
						if ch.Chan == channel {
							reply_msg = get_insult()
						} else {
							sender_state := c.StateTracker().GetNick(sender)
							_, exists := sender_state.Channels[ch.Chan]
							if exists {
								c.Privmsg(channel, seen_user+" is on "+ch.Chan)
							} else {
								c.Privmsg(channel, "Yeah, somewhere... can't tell you where though.")
							}
						}
					}
				}
			}
			time_seen := time.Time{}
			data, found := userdata[seen_user]
			if !found {
				c.Privmsg(channel, get_user_not_exists())
				return
			}
			fmt.Println("finding latest event/msg...")
			action := ""
			evt := Event{}
			msg := Message{}

			if len(data.Events) > 0 {
				evt = data.Events[len(data.Events)-1]
			}
			if len(data.Messages) > 0 {
				msg = data.Messages[len(data.Messages)-1]
			}
			is_event := false
			if evt.Event != "" {
				time_seen = evt.Timestamp
				is_event = true
			} else if msg.Msg != "" {
				time_seen = msg.Timestamp
				is_event = false
			} else {
				if evt.Timestamp.Unix() > msg.Timestamp.Unix() {
					time_seen = evt.Timestamp
					is_event = true
				} else {
					time_seen = msg.Timestamp
					is_event = false
				}
			}

			if is_event {
				if evt.Event == "quit" {
					action = "quitting"
				}
				if evt.Event == "join" {
					action = "joining"
				}
			} else {
				action = "writing: \"" + msg.Msg + "\""
			}
			//fmt.Println(fmt.Sprintf("found latest event %s at %d", action, time_seen.Unix()))

			duration := time.Since(time_seen)
			days := 0
			hours := 0
			sec := int(duration.Seconds())
			min := int(duration.Minutes())
			if duration.Hours() > 0 {
				sec = 0
			}
			if duration.Hours() > 24 {
				days = int(duration.Hours()) / 24
				hours = int(duration.Hours()) % 24
				min = 0
			}
			if duration.Minutes() > 60 {
				hours = int(duration.Minutes()) / 60
				min = int(duration.Minutes()) % 60
			}

			times := []string{}
			if days > 0 {
				times = append(times, strconv.Itoa(days)+" day(s)")
			}
			if hours > 0 {
				times = append(times, strconv.Itoa(hours)+" hour(s)")
			}
			if min > 0 {
				times = append(times, strconv.Itoa(min)+" minute(s)")
			}
			if sec > 0 {
				times = append(times, strconv.Itoa(sec)+" second(s)")
			}

			time_str := strings.Join(times, ", ")
			c.Privmsg(channel, fmt.Sprintf("%s was last seen %s ago %s.", seen_user, time_str, action))
		}*/

		if is_command(text, cmd_game) {
			//query := strings.Replace(text, cmd_game, "")
			query := ""
			for i := 1; i < len(args); i++ {
				query += " " + args[i]
			}
			games, success := games.FindGames(query, "Mozilla/5.0 (Windows NT 10.0; WOW64; rv:43.0) Gecko/20100101 Firefox/43.0")
			if success {
				reply_msg = fmt.Sprintf("%s (%s) - %s\n", games[0].Name, games[0].Year, games[0].Url)
			}
		}

		if is_command(text, cmd_url) {
			var url Url
			var urls []Url
			urls = urldata

			is_cmd_last := args[1] == "last" || args[1] == "l"
			is_cmd_random := args[1] == "random" || args[1] == "r"
			is_cmd_find := args[1] == "find" || args[1] == "f"

			if is_cmd_last || is_cmd_random && len(args) == 3 {
				user_urls := userdata[args[2]].Urls
				if user_urls != nil {
					urls = user_urls
				}
			}
			if is_cmd_last {
				url = urls[len(urls)-1]
			}
			if is_cmd_random {
				url = urls[rand_int(0, len(urls)-1)]
			}
			if is_cmd_find {
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
			reply_msg = fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d] %v", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), url.Url)
		}
		if is_command(text, cmd_msg) {
			var msg Message
			var msgs []Message

			msgs = msgdata

			is_cmd_last := args[1] == "last" || args[1] == "l"
			is_cmd_random := args[1] == "random" || args[1] == "r"
			is_cmd_find := args[1] == "find" || args[1] == "f"

			if is_cmd_last || is_cmd_random && len(args) == 3 {
				user_msgs := userdata[args[2]].Messages
				if user_msgs != nil {
					msgs = user_msgs
				}
			}

			if is_cmd_last {
				msg = msgs[len(msgs)-2]
			}
			if is_cmd_random {
				msg = msgs[rand_int(0, len(msgs)-1)]
			}
			if is_cmd_find {
				expr := ""
				for i := 2; i < len(args); i++ {
					add := args[i]
					if i != 2 {
						add = " " + add
					}
					expr += add
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
			reply_msg = fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d] %v: %v", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), msg.User, msg.Msg)
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
			if min > max {
				return
			}
			reply_msg = "What about... " + strconv.Itoa(rand_int(int(min), int(max)))
		}
		if is_command(text, cmd_steam) {
			if len(args) < 2 {
				return
			}
			subcommand := args[1]
			success := false
			steam_appid := 0
			var err error

			if subcommand == "latest" || subcommand == "l" {
				steam_appid, success = steam.SearchSteampowered("http://store.steampowered.com/search/?sort_by=Released_DESC&tags=-1&category1=998&page=1", 0)
			}
			if subcommand == "random" || subcommand == "r" {
				page := strconv.Itoa(rand_int(1, 286))
				steam_appid, success = steam.SearchSteampowered("http://store.steampowered.com/search/?sort_by=Released_DESC&tags=-1&category1=998&page="+page, -2)
			}
			if subcommand == "trending" || subcommand == "t" {
				apps, suc := steam.GetTrending("Mozilla/5.0 (Windows NT 10.0; WOW64; rv:43.0) Gecko/20100101 Firefox/43.0")
				if suc {
					app := apps[0]
					str := fmt.Sprintf("[Steamcharts] %s [%s increase in players last 24h] %d current players. Type '.s a %d' to get more info.", app.Name, app.Increase, app.Players, app.Id)
					reply_msg = str
				}
			}

			if subcommand == "appid" || subcommand == "a" {
				steam_appid, err = strconv.Atoi(args[2])
				success = (err == nil)
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
				steam_appid, success = steam.SearchSteampowered(search_url, 0)
			}
			if success {
				fmt.Println(fmt.Sprintf("Found appid %d, retrieving info...", steam_appid))
				app, success2 := steam.GetAppInfo(steam_appid, config.UserAgent)
				if success2 {
					rating_str := ""
					if app.Reviews > 0 {
						rating_str = fmt.Sprintf("| %.1f%s rating (%d reviews)", app.Rating, "%", app.Reviews)
					}

					os_str := ""
					if app.OS("") != "" {
						os_str = fmt.Sprintf("%s - [%s]", app.OS("/"), app.Features("/"))
					}
					price := ""
					if app.PriceDiscount != "" {
						price = "| " + app.PriceDiscount
					} else {
						if app.Price != "" {
							price = "| " + app.Price
						}
					}
					base_str := ""
					if app.ReleaseYear != "" && app.Developer != "" {
						base_str = fmt.Sprintf("(%s by \"%s\")", app.ReleaseYear, app.Developer)
					}

					info := fmt.Sprintf("[http://steamspy.com/app/%d/] \"%s\" %s %s %s %s", app.Id, app.Name, base_str, os_str, rating_str, price)
					reply_msg = info
					fmt.Println(info)
				} else {
					fmt.Println("Failed to retrieve steamapp info.")
				}

			} else {
				fmt.Println("Failed to retrieve appid from search.")
			}
		}
		// Handle URLs
		if !(sender == "Wipe" && (strings.Contains(text, "Steam") || strings.Contains(text, "YouTube"))) {
			fmt.Println("Looking for URLs...")
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
					reply_msg = reddit
				} else {
					fmt.Println("Failed to retrieve reddit URL for the link.")
				}
				last_url = url
			}
		}
		if reply_msg != "" {
			privmsg.Trailing = reply_msg
			privmsg.Params = []string{reply_ch}
			sendmsg = privmsg
		}
	}
	if sendmsg != nil {
		irc_writer.Encode(sendmsg)
		fmt.Println("Sending: " + sendmsg.String())
	}
	time.Sleep(1 * time.Millisecond)
}
