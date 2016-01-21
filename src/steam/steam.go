package steam

import (
	"github.com/op/go-logging"
	"golang.org/x/net/html"
	"io/ioutil"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var log = logging.MustGetLogger("steam")

type Trending struct {
	Id       int
	Name     string
	Increase string
	Players  int
}

type SteamApp struct {
	Id            int
	AppType       string
	Name          string
	Developer     string
	Publisher     string
	ReleaseDate   string
	ReleaseYear   string
	Price         string
	PriceDiscount string
	Rating        float32
	Reviews       int
	InGame        int
	Achievements  bool
	Linux         bool
	Windows       bool
	OSX           bool
	SinglePlayer  bool
	MultiPlayer   bool
	MMO           bool
	VAC           bool
	EarlyAccess   bool
	SteamCloud    bool
	Coop          bool
	Workshop      bool
	TradingCards  bool
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

type Matcher func(node *html.Node) bool

func html_find_all(node *html.Node, matcher Matcher) []*html.Node {
	if matcher(node) {
		return []*html.Node{node}
	}

	matched := []*html.Node{}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		found := html_find_all(c, matcher)
		if len(found) > 0 {
			matched = append(matched, found...)
		}
	}
	return matched
}

func get_appinfo_steampowered(appid int, useragent string) (SteamApp, bool) {
	s_appid := strconv.Itoa(appid)
	app := SteamApp{}
	app.Id = appid

	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://store.steampowered.com/app/"+s_appid+"/", nil)
	if err != nil {
		log.Error(err.Error())
		return app, false
	}
	req.Header.Set("User-Agent", useragent)
	resp, err := client.Do(req)
	if err != nil {
		log.Error(err.Error())
		return app, false
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err.Error())
		return app, false
	}
	s_body := string(body)
	s_body_nocr := strings.Replace(s_body, "\n", "", -1)

	re_name := regexp.MustCompile("<span itemprop=\"name\">(.+?)</span>")
	re_releasedate := regexp.MustCompile("<span class=\"date\">(.+?)</span>")
	release := re_releasedate.FindStringSubmatch(s_body)
	if release != nil {
		date := strings.Replace(release[1], ",", "", -1)
		date_p := strings.Split(date, " ")
		app.ReleaseDate = release[1]
		app.ReleaseYear = date_p[2]
	} else {
		log.Debug("Unable to parse release date.")
	}

	name := re_name.FindStringSubmatch(s_body)
	if name != nil {
		app.Name = name[1]
	}

	// Parse rating
	re_rating := regexp.MustCompile("(\\d+?\\.*\\d+?)% of the (\\d+,*\\d*?) user reviews for this game")
	re_rating_m := re_rating.FindStringSubmatch(s_body)
	if re_rating_m != nil {
		log.Debug(re_rating_m[0])
		f_rating, _err := strconv.ParseFloat(re_rating_m[1], 32)
		if _err == nil {
			app.Rating = float32(f_rating)
		}
		i_reviews, _err := strconv.Atoi(strings.Replace(re_rating_m[2], ",", "", -1))
		if _err == nil {
			app.Reviews = i_reviews
		}
	}
	re_dev := regexp.MustCompile("\\?developer.+\">(.+?)</a>")
	re_pub := regexp.MustCompile("\\?publisher.+\">(.+?)</a>")
	re_price := regexp.MustCompile("<div class=\"game_purchase_price price\">(.+?)</div>")
	re_price_orig := regexp.MustCompile("<div class=\"discount_original_price\">(.+?)</div>")
	re_price_discount := regexp.MustCompile("<div class=\"discount_final_price\">(.+?)</div>")

	price := re_price.FindStringSubmatch(s_body_nocr)
	price_orig := re_price_orig.FindStringSubmatch(s_body)
	price_discount := re_price_discount.FindStringSubmatch(s_body)
	if price != nil {
		app.Price = strings.TrimSpace(price[1])
	}
	if price_orig != nil {
		app.Price = strings.TrimSpace(price_orig[1])
	}
	if price_discount != nil {
		app.PriceDiscount = strings.TrimSpace(price_discount[1])
	}

	dev := re_dev.FindStringSubmatch(s_body)
	if dev != nil {
		app.Developer = html.UnescapeString(dev[1])
	}
	pub := re_pub.FindStringSubmatch(s_body)
	if pub != nil {
		app.Publisher = html.UnescapeString(dev[1])
	}

	// OS
	app.Linux = strings.Contains(s_body, "platform_img linux")
	app.Windows = strings.Contains(s_body, "platform_img win")
	app.OSX = strings.Contains(s_body, "platform_img mac")

	// Features
	app.SteamCloud = strings.Contains(s_body, ">Steam Cloud</a>")
	app.SinglePlayer = strings.Contains(s_body, ">Single-player</a>")
	app.MultiPlayer = strings.Contains(s_body, ">Multi-player</a>")
	app.Coop = strings.Contains(s_body, ">Local Co-op</a>")
	app.MMO = strings.Contains(s_body, ">MMO</a>")
	app.VAC = strings.Contains(s_body, ">Valve Anti-Cheat enabled</a>")
	app.EarlyAccess = strings.Contains(s_body, "<h1 class=\"inset\">Early Access Game</h1>")
	app.TradingCards = strings.Contains(s_body, ">Steam Trading Cards</a>")
	app.Achievements = strings.Contains(s_body, ">Steam Achievements</a>")
	app.Workshop = strings.Contains(s_body, ">Steam Workshop</a>")

	return app, true
}

func get_appinfo_steamdb(appid int, useragent string) (SteamApp, bool) {
	s_appid := strconv.Itoa(appid)
	app := SteamApp{}
	app.Id = appid

	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://steamdb.info/app/"+s_appid+"/info/", nil)
	if err != nil {
		log.Error(err.Error())
		return app, false
	}
	req.Header.Set("User-Agent", useragent)
	resp, err := client.Do(req)
	if err != nil {
		log.Error(err.Error())
		return app, false
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err.Error())
		return app, false
	}
	s_body := string(body)
	s_body = strings.Replace(s_body, "\n", "", -1)
	re := regexp.MustCompile("<table class=\"table table-bordered table-hover table-dark\">(.+?)</table>")
	match := re.FindStringSubmatch(s_body)
	if match == nil {
		log.Debug("Unable to find table.")
		return app, false
	} else {
		//fmt.Println(match[1])
	}
	table := match[1]

	// Parse release date
	re_releasedate := regexp.MustCompile("Release Date</td><td>(.+?)<i")
	re_inner := regexp.MustCompile("<.*?>(.+?)<")
	re_cells := regexp.MustCompile("<td.*?>(.+?)</td>")
	cells := re_cells.FindAllStringSubmatch(table, -1)
	release := re_releasedate.FindStringSubmatch(s_body)
	if release != nil {
		date := strings.Replace(release[1], ",", "", -1)
		date_p := strings.Split(date, " ")
		app.ReleaseDate = release[1]
		app.ReleaseYear = date_p[2]
	} else {
		log.Debug("Unable to parse release date.")
	}

	// Parse rating
	re_rating := regexp.MustCompile("(\\d+?\\.*\\d+?)% of the (\\d+,*\\d*?) user reviews for this game")
	re_rating_m := re_rating.FindStringSubmatch(s_body)
	if re_rating_m != nil {
		log.Debug(re_rating_m[0])
		f_rating, _err := strconv.ParseFloat(re_rating_m[1], 32)
		if _err == nil {
			app.Rating = float32(f_rating)
		}
		i_reviews, _err := strconv.Atoi(strings.Replace(re_rating_m[2], ",", "", -1))
		if _err == nil {
			app.Reviews = i_reviews
		}
	}
	for i, cell := range cells {
		content := ""
		if i != len(cells)-1 {
			content = cells[i+1][1]
			content = strings.Replace(content, "&reg;", "", -1)
			content = strings.TrimSpace(content)
		}
		if strings.Contains(cell[1], "App Type") {
			app.AppType = content
		}
		if strings.Contains(cell[1], "Name") && !strings.Contains(cell[1], "Store") { // discard "Store Name"
			app.Name = html.UnescapeString(content)
		}
		if strings.Contains(cell[1], "Developer") {
			dev := re_inner.FindStringSubmatch(content)
			if dev != nil {
				app.Developer = strings.TrimSpace(html.UnescapeString(dev[1]))
			}
		}
		if strings.Contains(cell[1], "Publisher") {
			publisher := re_inner.FindStringSubmatch(content)
			if publisher != nil {
				app.Publisher = strings.TrimSpace(html.UnescapeString(publisher[1]))
			}
		}
	}

	// OS
	app.Linux = strings.Contains(table, "icon-linux")
	app.Windows = strings.Contains(table, "icon-windows")
	app.OSX = strings.Contains(table, "icon-macos")

	// Features
	app.SteamCloud = strings.Contains(s_body, "aria-label=\"Steam Cloud\"")
	app.SinglePlayer = strings.Contains(s_body, "aria-label=\"Single-player\"")
	app.MultiPlayer = strings.Contains(s_body, "aria-label=\"Multi-player\"")
	app.Coop = strings.Contains(s_body, "aria-label=\"Co-op\"")
	app.MMO = strings.Contains(s_body, "aria-label=\"MMO\"")
	app.VAC = strings.Contains(s_body, "aria-label=\"Valve Anti-Cheat enabled\"")
	app.EarlyAccess = strings.Contains(s_body, "aria-label=\"Early Access\"")
	app.TradingCards = strings.Contains(s_body, "aria-label=\"Steam Trading Cards\"")
	app.Achievements = strings.Contains(s_body, "aria-label=\"Steam Achievements\"")
	app.Workshop = strings.Contains(s_body, "aria-label=\"Steam Workshop\"")

	log.Debug("Done collecting info.")

	return app, true
}

// Search on steampowered.com
func SearchSteampowered(url string, index int) (int, bool) {
	resp, err := http.Get(url)
	if err != nil {
		return -1, false
	}
	root, err := html.Parse(resp.Body)
	if err != nil {
		return -1, false
	}

	matcher := func(n *html.Node) bool {
		for _, a := range n.Attr {
			if a.Key == "class" {
				if a.Val == "responsive_search_name_combined" {
					return true
				}
			}
		}
		return false
	}

	games := html_find_all(root, matcher)
	if games == nil || len(games) == 0 {
		return -1, false
	}
	// Last game
	if index == -1 {
		index = len(games) - 1
	} else if index == -2 {
		rand.Seed(time.Now().Unix())
		index = rand.Intn(len(games) - 1)
	}

	game := games[index]
	href := game.Parent
	appid := 0
	re := regexp.MustCompile("/app/(\\d+?)/")
	link := ""
	for _, a := range href.Attr {
		if a.Key == "href" {
			link = a.Val
		}
	}

	app_s := re.FindStringSubmatch(link)
	if app_s != nil {
		appid, _ = strconv.Atoi(app_s[1])
	} else {
		return -1, false
	}
	return appid, true
}

func GetTrending(useragent string) (games_result []Trending, success bool) {
	games := []Trending{}

	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://steamcharts.com/", nil)
	if err != nil {
		log.Error(err.Error())
		return games, false
	}
	req.Header.Set("User-Agent", useragent)
	resp, err := client.Do(req)
	if err != nil {
		log.Error(err.Error())
		return games, false
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Error(err.Error())
		return games, false
	}
	s_body := string(body)
	s_body = strings.Replace(s_body, "\n", "", -1)

	re := regexp.MustCompile("td class=\"game-name left\">.+?href=\"/app/(\\d+?)\">(.+?)</a>.+?\"gain\">(.+?)</td>.+? class=\"num\">(.+?)</td>")
	matches := re.FindAllStringSubmatch(s_body, -1)
	if matches != nil {
		for _, match := range matches {
			log.Debugf("Found match: %s, %s, %s, %s", match[1], match[2], match[3], match[4])
			app_s := match[1]
			name := strings.TrimSpace(match[2])
			gain := html.UnescapeString(match[3])
			num_s := strings.Replace(match[4], ",", "", -1)

			app, _ := strconv.Atoi(app_s)
			num, _ := strconv.Atoi(num_s)

			games = append(games, Trending{app, name, gain, num})
		}
		return games, true
	}
	return games, false
}

func GetAppInfo(appid int, useragent string) (SteamApp, bool) {
	app, result := get_appinfo_steampowered(appid, useragent)
	// fallback methods
	if app.Name == "" || !result {
		log.Debug("Unable to find appinfo on steampowered, using steamdb as fallback.")
		app, result = get_appinfo_steamdb(appid, useragent)
		if app.Name == "" || !result {
			return app, false
		}
	}
	return app, true
}
