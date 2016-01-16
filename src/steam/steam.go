package steam

import (
	"fmt"
	"golang.org/x/net/html"
	"io/ioutil"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

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
	SteamCloud   bool
	Coop         bool
	Workshop     bool
	TradingCards bool
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

/*func get_appinfo_steampowered(appid int, useragent string) (SteamApp, bool) {
	s_appid := strconv.Itoa(appid)
	app := SteamApp{}
	app.Id = appid

	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://store.steampowered.com/app/"+s_appid+"/", nil)
	if err != nil {
		fmt.Println(err.Error())
		return app, false
	}
	req.Header.Set("User-Agent", useragent)
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
}*/

func get_appinfo_steamdb(appid int, useragent string) (SteamApp, bool) {
	s_appid := strconv.Itoa(appid)
	app := SteamApp{}
	app.Id = appid

	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://steamdb.info/app/"+s_appid+"/info/", nil)
	if err != nil {
		fmt.Println(err.Error())
		return app, false
	}
	req.Header.Set("User-Agent", useragent)
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
	re := regexp.MustCompile("<table class=\"table table-bordered table-hover table-dark\">(.+?)</table>")
	match := re.FindStringSubmatch(s_body)
	if match == nil {
		fmt.Println("Unable to find table.")
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
		fmt.Println("Unable to parse release date.")
	}

	// Parse rating
	re_rating := regexp.MustCompile("(\\d+?\\.*\\d+?)% of the (\\d+,*\\d*?) user reviews for this game")
	re_rating_m := re_rating.FindStringSubmatch(s_body)
	if re_rating_m != nil {
		fmt.Println(re_rating_m[0])
		f_rating, _err := strconv.ParseFloat(re_rating_m[1], 32)
		if _err == nil {
			app.Rating = float32(f_rating)
		}
		i_reviews, _err := strconv.Atoi(strings.Replace(re_rating_m[2], ",", "", -1))
		if _err == nil {
			app.Reviews = i_reviews
		}
	}
	fmt.Println("Table cells:")
	for i, cell := range cells {
		content := ""
		if i != len(cells)-1 {
			content = cells[i+1][1]
			content = strings.Replace(content, "&reg;", "", -1)
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
				//fmt.Println("Found dev:" + dev[1])
				app.Developer = html.UnescapeString(dev[1])
			}
		}
		if strings.Contains(cell[1], "Publisher") {
			publisher := re_inner.FindStringSubmatch(content)
			if publisher != nil {
				//fmt.Println("Found publisher:" + publisher[1])
				app.Publisher = html.UnescapeString(publisher[1])
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

	fmt.Println("Done collecting info.")

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

func GetAppInfo(appid int, useragent string) (SteamApp, bool) {
	return get_appinfo_steamdb(appid, useragent)
}

func steam_test() {

}
