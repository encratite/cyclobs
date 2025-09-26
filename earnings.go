package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/encratite/commons"
)

type companyEDGARData struct {
	body string
	triggered bool
}

type companyInvestingData struct {
	symbol string
	eps string
	triggered bool
}

func runEarningsSystem() {
	loadConfiguration()
	var wg sync.WaitGroup
	wg.Add(2)
	go watchEDGAR(&wg)
	go watchInvesting(&wg)
	wg.Wait()
}

func watchEDGAR(wg *sync.WaitGroup) {
	companies := map[string]companyEDGARData{}
	keepWatching := true
	for keepWatching {
		for _, company := range configuration.Earnings {
			getEDGARData(company, &companies)
			keepWatching = false
			for _, data := range companies {
				if !data.triggered {
					keepWatching = true
				}
			}
			if !keepWatching {
				break
			}
			time.Sleep(time.Duration(1000) * time.Millisecond)
		}
	}
	log.Printf("Terminating EDGAR watcher")
	wg.Add(1)
}

func getEDGARData(company EarningsConfiguration, companies *map[string]companyEDGARData) {
	key := company.CIK
	data, exists := (*companies)[key]
	if exists && data.triggered {
		return
	}
	client := &http.Client{
		Transport: &http.Transport{},
		Timeout: 10 * time.Second,
	}
	url := fmt.Sprintf("https://data.sec.gov/submissions/CIK%s.json", company.CIK)
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create request (%s): %v", url, err)
		return
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:142.0) Gecko/20100101 Firefox/142.0")
	response, err := client.Do(request)
	if err != nil {
		log.Printf("Failed to GET data (%s): %v", url, err)
		return
	}
	defer response.Body.Close()
	bodyBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Failed to read all (%s): %v", url, err)
		return
	}
	body := string(bodyBytes)
	if !exists {
		data = companyEDGARData{
			body: body,
			triggered: false,
		}
	}
	if !data.triggered && body != data.body {
		log.Printf("Detected a change in the filings of %s (CIK %s)", company.Symbol, company.CIK)
		beep()
		data.triggered = true
	}
	(*companies)[key] = data
}

func watchInvesting(wg *sync.WaitGroup) {
	data := []companyInvestingData{}
	for _, company := range configuration.Earnings {
		investingData := companyInvestingData{
			symbol: company.Symbol,
			eps: "--",
			triggered: false,
		}
		data = append(data, investingData)
	}
	for {
		getInvestingData(&data)
		keepWatching := commons.ContainsFunc(data, func (investingData companyInvestingData) bool {
			return !investingData.triggered
		})
		if !keepWatching {
			break
		}
		time.Sleep(time.Duration(10) * time.Second)
	}
	log.Printf("Terminating investing.com watcher")
	wg.Add(1)
}

func getInvestingData(data *[]companyInvestingData) {
	url := "https://www.investing.com/earnings-calendar/"
	response, err := http.Get(url)
	if err != nil {
		log.Printf("Failed to GET data (%s): %v", url, err)
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		log.Printf("Failed to read response (%s): %v", url, err)
		return
	}
	html := string(body)
	reader := strings.NewReader(html)
	doc, err := htmlquery.Parse(reader)
	if err != nil {
		log.Printf("Failed to parse HTML: %v", err)
		return
	}
	table := htmlquery.FindOne(doc, "//table[@id='earningsCalendarData']")
	if table == nil {
		log.Printf("Failed to find table")
		return
	}
	rows := htmlquery.Find(table, "/tbody/tr")
	if rows == nil {
		log.Printf("Failed to find rows")
		return
	}
	for _, row := range rows {
		link := htmlquery.FindOne(row, "//a[@class='bold middle']")
		if link == nil {
			continue
		}
		symbol := htmlquery.InnerText(link)
		epsNode := htmlquery.FindOne(row, "/td[3]")
		eps := htmlquery.InnerText(epsNode)
		i := slices.IndexFunc(*data, func (investingData companyInvestingData) bool {
			return investingData.symbol == symbol
		})
		if i == -1 {
			continue
		}
		investingData := &(*data)[i]
		if !investingData.triggered && eps != investingData.eps {
			log.Printf("Detected investing.com EPS for %s: %s", symbol, eps)
			beep()
			investingData.eps = eps
			investingData.triggered = true
		}
	}
}