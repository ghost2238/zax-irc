package main

import (
	"encoding/json"
	"fmt"
	irc "github.com/fluffle/goirc/client"
	"os"
	"strings"
)

type Channel struct {
	Chan     string
	Password string
}

type Config struct {
	Admin      string
	Username   string
	Nickname   string
	Server     string
	ReportChan string
	Channels   []Channel
}

func main() {
	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	config := Config{}
	err := decoder.Decode(&config)
	if err != nil {
		fmt.Println("error:", err)
	}

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

	c.HandleFunc(irc.PRIVMSG,
		func(conn *irc.Conn, line *irc.Line) {

			p := strings.Split(line.Src, "!")
			sender := p[0]

			if len(config.ReportChan) > 0 {
				c.Privmsg(config.ReportChan, "["+line.Target()+"] "+sender+": "+line.Text())
			}

			if sender == config.Admin {
				if line.Text() == "die" {
					c.Quit("As you say.")
				}
			}

		})

	if err := c.Connect(); err != nil {
		fmt.Printf("Connection error: %s\n", err.Error())
	}

	<-quit
}
