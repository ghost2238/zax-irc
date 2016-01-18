package games

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
)

func mobygames_search(url string, result_needed string, useragent string) (page string, success bool) {

	fmt.Println(url)
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err.Error())
		return "", false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; WOW64; rv:43.0) Gecko/20100101 Firefox/43.0")
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error())
		return "", false
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err.Error())
		return "", false
	}
	s_body := string(body)
	fmt.Println(resp.StatusCode)
	fmt.Println(resp.Status)
	fmt.Println(len(s_body))

	if result_needed == "Developer" {
		re_dev := regexp.MustCompile("<a href=\"http://www.mobygames.com/developer/sheet/view/developerId,(\\d+?)/\">(.+?)</a>")
		dev_page := re_dev.FindStringSubmatch(s_body)
		if dev_page != nil {
			//fmt.Printf("url: %s for %s", dev_page[1], dev_page[2])
			return "http://www.mobygames.com/developer/sheet/view/developerId," + dev_page[1], true
		}
	}
	//fmt.Println(body)
	return "", false
}

type GameResult struct {
	Name string
	Year string
	Url  string
}

func FindGames(query string, useragent string) (found_games []GameResult, found bool) {
	author := ""
	role := ""
	year := ""
	query = strings.ToLower(query)

	created_roles := "crafted|made|created"
	code_roles := "code|coded|programming"
	design_roles := "designed|design"
	production_roles := "produced|production"
	art_roles := "artwork|art"
	sound_roles := "soundtrack|sound|music"

	possible_roles := "(" + created_roles + "|" + code_roles + "|" + production_roles + "|" + design_roles + "|" + art_roles + "|" + sound_roles + ")"
	re_query := regexp.MustCompile(possible_roles + " by (.+?) (?:in|from) (\\d+)")
	year_query := regexp.MustCompile(possible_roles + "in the year (\\d+)")
	match := year_query.FindStringSubmatch(query)
	if match != nil {
		year = match[1]
	}

	match = re_query.FindStringSubmatch(query)
	if match != nil {
		role = match[1]
		author = match[2]
		year = match[3]
		fmt.Println("match1")
	} else {
		re_query = regexp.MustCompile(possible_roles + " by (.+)")
		match := re_query.FindStringSubmatch(query)
		if match != nil {
			fmt.Println("match2")
			role = match[1]
			author = match[2]
		} else {
			re_query = regexp.MustCompile("by (.+) (?:in|from) (\\d+)")
			match = re_query.FindStringSubmatch(query)
			if match != nil {
				role = "created"
				author = match[1]
				year = match[2]
				fmt.Println("match3")
			}
		}
	}

	if role == "crafted" || role == "made" {
		role = "created"
	}

	if role == "soundtrack" || role == "music" {
		role = "sound"
	}

	if role == "produced" {
		role = "production"
	}

	if role == "artwork" {
		role = "art"
	}

	if role == "designed" || role == "" {
		role = "design"
	}

	if role == "code" || role == "coded" {
		role = "programming"
	}

	games := []GameResult{}

	fmt.Println("year: " + year)
	fmt.Println("author:" + author)
	fmt.Println("role:" + role)

	//return nil, false

	if author == "" {
		return nil, false
	}
	url_author := strings.Replace(author, " ", "+", -1)
	url, found := mobygames_search("https://www.mobygames.com/search/quick?q="+url_author+"&p=-1&search=Go&sFilter=1&sD=on", "Developer", useragent)
	//url, found := mobygames_search("https://www.mobygames.com/", "Developer", useragent)
	//url := "https://www.mobygames.com/developer/sheet/view/developerId,682/"
	if !found {
		fmt.Println("Unable to find URL.")
		return nil, false
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println(err.Error())
		return games, false
	}
	req.Header.Set("User-Agent", useragent)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error())
		return games, false
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err.Error())
		return games, false
	}
	s_body := string(body)

	re_credit := regexp.MustCompile("<a href=\"(.+?)\">(.+?)</a> \\((.+?)\\).+devCreditsDivider.+devCreditsTitle\">(.+?)</span>")
	dev_page := re_credit.FindAllStringSubmatch(s_body, -1)
	if dev_page != nil {
		for _, entry := range dev_page {
			r_role := entry[4]
			rl_role := strings.ToLower(r_role)
			r_url := "https://www.mobygames.com" + entry[1]
			r_year := entry[3]
			r_title := entry[2]

			if year != "" && year != r_year {
				continue
			}

			if role == "production" {
				if rl_role != "producer" && rl_role != "produced" && rl_role != "producers" && rl_role != "executive producer" {
					continue
				}
			}

			if role == "sound" {
				if rl_role != "music" && rl_role != "additional music" && rl_role != "composed by" && rl_role != "music by" &&
					rl_role != "audio team" {
					continue
				}
			}

			if role == "art" {
				if rl_role != "artists" && rl_role != "art" && rl_role != "computer artist" {
					continue
				}
			}

			if role == "design" {
				if rl_role != "designer" && rl_role != "design" && rl_role != "game design" {
					continue
				}
			}
			if role == "programming" {
				if rl_role != "additional programming" && rl_role != "programming" && rl_role != "programmers" &&
					rl_role != "3d engine" && rl_role != "engine programmer" && rl_role != "lead programmer" &&
					rl_role != "bot ai and additional programming" && rl_role == "bot ai" && rl_role == "bot ai by" {
					continue
				}
			}
			games = append(games, GameResult{r_title, r_year, r_url})
		}
		return games, true
	}
	return games, false
}
