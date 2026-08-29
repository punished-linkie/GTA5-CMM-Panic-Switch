package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func (g *HeistGUI) startDuckDNSUpdater() {
	for {
		if g.DuckDomain != "" && g.DuckToken != "" && g.DuckDomain != "yourname" {
			ipResp, err := http.Get("https://api6.ipify.org")
			if err != nil {
				time.Sleep(5 * time.Minute)
				continue
			}
			ipv6Bytes, _ := io.ReadAll(ipResp.Body)
			ipResp.Body.Close()
			myIPv6 := strings.TrimSpace(string(ipv6Bytes))

			url := fmt.Sprintf("https://www.duckdns.org/update?domains=%s&token=%s&ipv6=%s&verbose=true", g.DuckDomain, g.DuckToken, myIPv6)
			resp, err := http.Get(url)
			if err == nil {
				resp.Body.Close()
			}
		}
		time.Sleep(5 * time.Minute)
	}
}
