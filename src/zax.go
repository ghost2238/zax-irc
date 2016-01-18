package main

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"games"
	client "github.com/fluffle/goirc/client"
	irc "github.com/fluffle/goirc/client"
	irc_logging "github.com/fluffle/goirc/logging"
	"github.com/mvdan/xurls"
	"github.com/op/go-logging"
	"math/rand"
	"os"
	"reddit"
	"regexp"
	"steam"
	"strconv"
	"strings"
	"time"
)

var log = logging.MustGetLogger("zax")
var format = logging.MustStringFormatter(
	`%{color}%{time:15:04:05.000} %{longfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
)

type ChannelCredentials struct {
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

type Config struct {
	Admin             string // nick:<expr> | host:<expr>
	Username          string
	Nickname          string
	Server            string
	ReportChan        string
	UserAgent         string
	SSL               bool
	SSLIgnoreInsecure bool
	Channels          []ChannelCredentials
	Handlers          []string
	News              []string
}

type ZAX struct {
	IrcConfig *client.Config
	IrcClient *client.Conn
}

// History
var msgdata []Message
var eventdata []Event
var urldata []Url
var userdata map[string]*User
var file_history *os.File
var file_history_writer *bufio.Writer

var ignore_list []string // Ignore these usernames

var config Config

var last_url string

// for context control.
var last_context_index int
var last_context_type string

var zax ZAX

func (zax ZAX) Privmsg(t, msg string) {
	log.Debugf("Zax::Privmsg: [%s] %s", t, msg)
	zax.IrcClient.Privmsg(t, msg)
}

func (zax ZAX) Quit(msg string) {
	log.Debugf("Zax::Quit: %s", msg)
	zax.IrcClient.Quit(msg)
}

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
	msg := []string{"Uh, never mind.", "This system is too advanced for you.", "That was an illogical decision.",
		"Weeeeeeeeeeeeeeeeeeeeee[bzzt]", "Didn't we have some fun, though?", "Your entire life has been a mathematical error."}
	return msg[rand_int(0, len(msg))]
}

func get_user_not_exists() string {
	msg := []string{"Who?", "Never heard of that human.", "Negative, no record found", "Did you spell that correctly?", "Are you sober?"}
	return msg[rand_int(0, len(msg))]
}

func get_insult() string {
	msg := []string{"What do you think you are doing?", "You're not a good person. You know that, right?", "Typical human.",
		"You don't even care. Do you?", "I guess we both know that isn't going to happen.", "All right, keep doing whatever it is you think you're doing.", "Are you sober?"}
	return msg[rand_int(0, len(msg))]
}

// Wrapper that let's IRC library log via go-logging.
type IrcLogger struct {
}

func (logger IrcLogger) Debug(f string, args ...interface{}) { log.Debugf(f, args) }
func (logger IrcLogger) Info(f string, args ...interface{})  { log.Infof(f, args) }
func (logger IrcLogger) Warn(f string, args ...interface{})  { log.Warningf(f, args) }
func (logger IrcLogger) Error(f string, args ...interface{}) { log.Errorf(f, args) }

func main() {

	zax = ZAX{}
	zax_log, _ := os.OpenFile("zax.log", os.O_CREATE|os.O_APPEND, os.ModeAppend)
	irc_logging.SetLogger(IrcLogger{})

	log_file := logging.NewLogBackend(zax_log, "", 0)
	log_file_f := logging.NewBackendFormatter(log_file, format)

	log_stdout := logging.NewLogBackend(os.Stdout, "", 0)
	log_stdout_f := logging.NewBackendFormatter(log_stdout, format)
	log_stdout_levelled := logging.AddModuleLevel(log_stdout_f)
	log_stdout_levelled.SetLevel(logging.INFO, "")

	logging.SetBackend(log_stdout_levelled, log_file_f)

	last_url = ""
	userdata = make(map[string]*User)
	log.Notice("Loading config...")

	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	config = Config{}
	err := decoder.Decode(&config)
	if err != nil {
		log.Errorf("Error loading config: %s", err.Error())
		os.Exit(-1)
	}

	log.Notice("Config loaded.")
	log.Notice("Opening history...")
	time_history := time.Now()
	file_history, err = os.OpenFile("history.log", os.O_CREATE, os.ModeAppend)
	if err != nil {
		log.Errorf("Unable to open history.log: %s", err.Error())
		os.Exit(-1)
	}
	log.Notice("Loading history...")
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
			//log.Debugf("[%d-%02d-%02d %02d:%02d:%02d]", timestamp.Year(), timestamp.Month(), timestamp.Day(), timestamp.Hour(), timestamp.Minute(), timestamp.Second())
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
	log.Noticef("History loaded %d events, %d urls and %d messages in %f seconds.\n", len(eventdata), len(urldata), len(msgdata), elapsed.Seconds())
	log.Notice("Initializing IRC connection.")

	// Init IRC connection
	log.Noticef("Creating IRC cfg. Server: %s, Use SSL: %t, Nickname %s", config.Server, config.SSL, config.Nickname)
	cfg := irc.NewConfig(config.Nickname)
	cfg.SSL = config.SSL
	cfg.SSLConfig = &tls.Config{}
	cfg.SSLConfig.InsecureSkipVerify = config.SSLIgnoreInsecure
	cfg.Me.Ident = "ZAX"
	cfg.Me.Name = "ZAX"
	cfg.Version = "ZAX"
	cfg.Server = config.Server
	cfg.NewNick = func(n string) string { return n + "^" }
	quit := make(chan bool)
	c := irc.Client(cfg)
	c.EnableStateTracking()
	c.Connect()
	zax.IrcClient = c
	zax.IrcConfig = cfg
	c.HandleFunc(irc.CONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			for i := 0; i < len(config.Channels); i++ {
				ch := config.Channels[i]
				c.Join(ch.Chan, ch.Password)
			}
		})
	c.HandleFunc(irc.DISCONNECTED,
		func(conn *irc.Conn, line *irc.Line) {
			log.Notice("Disconnected")
			quit <- true
		})
	c.HandleFunc(irc.JOIN,
		func(conn *irc.Conn, line *irc.Line) {
			log.Info("[" + line.Target() + "] " + line.Nick + " (" + line.Ident + "@" + line.Host + ") has joined.")
			history_event(line.Nick, "join", "", line.Target())
			if line.Nick == config.Nickname {
				return
			}
			time.Sleep(100 * time.Millisecond)
		})

	c.HandleFunc(irc.QUIT,
		func(conn *irc.Conn, line *irc.Line) {
			log.Info("[" + line.Target() + "] " + line.Nick + " (" + line.Ident + "@" + line.Host + ") has quit.")
			history_event(line.Nick, "quit", line.Text(), "")
			time.Sleep(100 * time.Millisecond)
		})

	c.HandleFunc(irc.PING,
		func(conn *irc.Conn, line *irc.Line) {
			log.Debug("PING.")
			time.Sleep(100 * time.Millisecond)
		})

	c.HandleFunc(irc.PRIVMSG,
		func(conn *irc.Conn, line *irc.Line) {
			sender_host := line.Host
			sender := line.Src
			text := line.Text()
			channel := line.Target()
			target := line.Target()
			reply_to := line.Target()

			if line.Target() == config.Nickname {
				reply_to = sender
			}
			// Remove control characters
			text = strings.Replace(text, "", "", -1)
			text = strings.Replace(text, "", "", -1)

			log.Notice("[" + line.Target() + "] " + sender + ": " + text)

			_, ok := userdata[sender]
			if !ok {
				m := []Message{}
				e := []Event{}
				u := []Url{}
				userdata[sender] = &User{m, e, u}
			}

			history_message(sender, target, text)
			if len(config.ReportChan) > 0 {
				zax.Privmsg(config.ReportChan, "["+target+"] "+sender+": "+text)
			}

			cmd_admin := []string{"%%", "<<"}

			// Check if admin
			if is_command(text, cmd_admin) {
				re_adm := regexp.MustCompile("(nick|host):(.+)")
				criteria := re_adm.FindStringSubmatch(config.Admin)
				//is_admin := false
				match_str := ""
				if criteria == nil {
					log.Debug("Unable to parse admin criteria.")
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
					log.Debugf("Didn't pass the criteria: %s:%s", criteria[1], criteria[2])
					return
				}
				log.Debugf("Passed the criteria: %s:%s with %s", criteria[1], criteria[2], match_str)
				if text == "<<" {
					zax.Quit(get_quit_msg())
				}
			}

			cmd_rand := []string{".r", ".random"}
			cmd_steam := []string{".s", ".steam"}
			cmd_game := []string{".g", ".game"}
			cmd_url := []string{".u", ".url"}
			cmd_msg := []string{".m", ".msg"}
			cmd_seen := []string{"!"}
			cmd_help := []string{"?h"}

			args := strings.Split(text, " ")
			if is_command(text, cmd_help) {
				reply_msg := ""
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
				zax.Privmsg(reply_to, reply_msg)
				return
			}
			if is_command(text, cmd_seen) {
				log.Debug("Executing seen command.")
				seen_user := strings.Replace(args[0], "!", "", -1)
				if seen_user == "" {
					log.Debug("No user was specified.")
					return
				}
				if seen_user == sender {
					log.Debug("Sender same as specified seen user, insult.")
					zax.Privmsg(reply_to, get_insult())
				}
				state := c.StateTracker().GetNick(seen_user)
				if state != nil {
					user_channels := state.Channels
					for i := 0; i < len(config.Channels); i++ {
						ch := config.Channels[i]
						_, exists := user_channels[ch.Chan]
						if exists {
							if ch.Chan == channel {
								zax.Privmsg(reply_to, get_insult())
								log.Notice("seen_user is here now.")
							} else {
								sender_state := c.StateTracker().GetNick(sender)
								_, exists := sender_state.Channels[ch.Chan]
								if exists {
									zax.Privmsg(reply_to, seen_user+" is on "+ch.Chan)
								} else {
									zax.Privmsg(reply_to, "Yeah, somewhere... can't tell you where though.")
								}
							}
						}
					}
				}
				time_seen := time.Time{}
				data, found := userdata[seen_user]
				if !found {
					zax.Privmsg(channel, get_user_not_exists())
					return
				}
				log.Debug("Finding latest event/msg...")
				action := ""
				evt := Event{}
				msg := Message{}

				if data.Events != nil && len(data.Events) > 0 {
					evt = data.Events[len(data.Events)-1]
				}
				if data.Messages != nil && len(data.Messages) > 0 {
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
				log.Debugf("Found latest event %s at %d", action, time_seen.Unix())

				duration := time.Since(time_seen)
				days := 0
				hours := 0
				sec := int(duration.Seconds())
				min := int(duration.Minutes())

				log.Debugf("User %s seen hours: %.1f, minutes: %.1f, seconds %.1f ago.", seen_user, duration.Hours(), duration.Minutes(), duration.Seconds())

				if duration.Hours() > 24 {
					days = int(duration.Hours()) / 24
					hours = int(duration.Hours()) % 24
					min = 0
				} else if duration.Minutes() > 60 {
					hours = int(duration.Hours()) / 60
					min = int(duration.Minutes()) % 60
				} else if duration.Seconds() > 60 {
					min = int(duration.Minutes())
					sec = int(duration.Seconds()) % 60
				} else {
					sec = int(duration.Seconds())
				}

				if duration.Hours() > 24 {
					days = int(duration.Hours()) / 24
					hours = int(duration.Hours()) % 24
					min = 0
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
				zax.Privmsg(reply_to, fmt.Sprintf("%s was last seen %s ago %s.", seen_user, time_str, action))
			}

			if is_command(text, cmd_game) {
				query := ""
				for i := 1; i < len(args); i++ {
					query += " " + args[i]
				}
				games, success := games.FindGames(query, config.UserAgent)
				if success {
					zax.Privmsg(reply_to, fmt.Sprintf("%s (%s) - %s\n", games[0].Name, games[0].Year, games[0].Url))
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
				zax.Privmsg(reply_to, fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d] %v", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), url.Url))
				return
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
				zax.Privmsg(reply_to, fmt.Sprintf("[%d-%02d-%02d %02d:%02d:%02d] %v: %v", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), msg.User, msg.Msg))
			}
			if is_command(text, cmd_rand) {
				if len(args) < 3 {
					return
				}
				min, err := strconv.ParseInt(args[1], 10, 32)
				if err != nil {
					log.Debug("Failed to parse min.")
					return
				}
				max, err := strconv.ParseInt(args[2], 10, 32)
				if err != nil {
					log.Debug("Failed to parse max.")
					return
				}
				if min > max {
					return
				}
				zax.Privmsg(reply_to, "What about... "+strconv.Itoa(rand_int(int(min), int(max))))
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
					apps, suc := steam.GetTrending(config.UserAgent)
					if suc {
						app := apps[0]
						zax.Privmsg(reply_to, fmt.Sprintf("[Steamcharts] %s [%s increase in players last 24h] %d current players. Type '.s a %d' to get more info.", app.Name, app.Increase, app.Players, app.Id))
						return
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
						log.Debug("Doesn't match.")
						return
					}
					log.Debugf("matched term: %s", match[1])
					search_url := "http://store.steampowered.com/search/?snr=&term=" + match[1]
					log.Debugf("Search URL: %s", search_url)
					steam_appid, success = steam.SearchSteampowered(search_url, 0)
				}
				if success {
					log.Info("Found appid %d, retrieving info...", steam_appid)
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
						zax.Privmsg(reply_to, info)
					} else {
						log.Error("Failed to retrieve steamapp info.")
					}

				} else {
					log.Notice("Failed to retrieve appid from search.")
				}
			}
			// Handle URLs
			if !(sender == "Wipe" && (strings.Contains(text, "Steam") || strings.Contains(text, "YouTube"))) {
				log.Debug("Looking for URLs...")
				urls := xurls.Relaxed.FindAllString(text, -1)
				for i := 0; i < len(urls); i++ {
					url := urls[i]
					log.Debugf("Found reddit url: %s", url)
					history_url(sender, url)

					if url == last_url {
						log.Debugf("Matches same url (%s) as last time, ignore.", last_url)
						continue
					}
					reddit, success := reddit.Search(url)
					if success {
						zax.Privmsg(reply_to, reddit)
					} else {
						log.Debug("Failed to retrieve reddit URL for the link.")
					}
					last_url = url
				}
			}
			time.Sleep(10 * time.Millisecond)
		})

	<-quit
	log.Notice("Closing history.log")
	file_history.Close()
	time.Sleep(1000 * time.Millisecond)
}
