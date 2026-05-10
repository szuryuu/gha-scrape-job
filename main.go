package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	graphqlURL = "https://id.jobstreet.com/graphql"
	dataFile   = "data/results.jsonl"
)

var keywords = []string{
	"sysadmin",
	"devops",
	"cloud",
	"azure",
	"infrastructure",
}

type City struct {
	Name  string
	Where string
}

var cities = []City{
	{Name: "national", Where: ""},
	{Name: "Jakarta", Where: "DKI Jakarta"},
	{Name: "Yogyakarta", Where: "DI Yogyakarta"},
	{Name: "Surabaya", Where: "Surabaya"},
}

const jobSearchQuery = `
query JobSearchV6($params: JobSearchV6QueryInput!) {
  jobSearchV6(params: $params) {
    totalCount
  }
}`

type graphqlRequest struct {
	OperationName string       `json:"operationName"`
	Variables     gqlVariables `json:"variables"`
	Query         string       `json:"query"`
}

type gqlVariables struct {
	Params gqlParams `json:"params"`
}

type gqlParams struct {
	Channel   string `json:"channel"`
	Keywords  string `json:"keywords"`
	Locale    string `json:"locale"`
	Page      int    `json:"page"`
	PageSize  int    `json:"pageSize"`
	SiteKey   string `json:"siteKey"`
	Source    string `json:"source"`
	SessionID string `json:"eventCaptureSessionId"`
	UserID    string `json:"eventCaptureUserId"`
	Where     string `json:"where,omitempty"`
}

type graphqlResponse struct {
	Data struct {
		JobSearchV6 struct {
			TotalCount int `json:"totalCount"`
		} `json:"jobSearchV6"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type Result struct {
	Keyword string `json:"keyword"`
	City    string `json:"city"`
	Count   int    `json:"count"` // -1 if error
	URL     string `json:"url"`
}

type Record struct {
	Date    string   `json:"date"`
	Results []Result `json:"results"`
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func buildJobURL(keyword string, city City) string {
	base := fmt.Sprintf("https://id.jobstreet.com/id/%s-jobs", keyword)
	if city.Where == "" {
		return base
	}
	slug := strings.ReplaceAll(city.Where, " ", "-")
	return fmt.Sprintf("%s/in-%s", base, slug)
}

func fetchCount(keyword string, city City, sessionID string) (int, error) {
	payload := graphqlRequest{
		OperationName: "JobSearchV6",
		Variables: gqlVariables{
			Params: gqlParams{
				Channel:   "web",
				Keywords:  keyword,
				Locale:    "id-ID",
				Page:      1,
				PageSize:  1,
				SiteKey:   "ID",
				Source:    "FE_SERP",
				SessionID: sessionID,
				UserID:    sessionID,
				Where:     city.Where,
			},
		},
		Query: jobSearchQuery,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", graphqlURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("seek-request-brand", "jobstreet")
	req.Header.Set("seek-request-country", "ID")
	req.Header.Set("x-custom-features", "application/features.seek.all+json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:144.0) Gecko/20100101 Firefox/144.0")
	req.Header.Set("Origin", "https://id.jobstreet.com")
	req.Header.Set("Referer", "https://id.jobstreet.com/")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		preview, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, string(preview))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	var gqlResp graphqlResponse
	if err := json.Unmarshal(respBody, &gqlResp); err != nil {
		return 0, fmt.Errorf("unmarshal: %w", err)
	}

	if len(gqlResp.Errors) > 0 {
		return 0, fmt.Errorf("graphql error: %s", gqlResp.Errors[0].Message)
	}

	return gqlResp.Data.JobSearchV6.TotalCount, nil
}

func appendRecord(record Record) error {
	if err := os.MkdirAll("data", 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(dataFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	if err := json.NewEncoder(w).Encode(record); err != nil {
		return err
	}
	return w.Flush()
}

func main() {
	log.SetFlags(log.Ltime)
	log.Println("==========================================")
	log.Printf("JobStreet Tracker - %s", time.Now().Format("2006-01-02"))
	log.Printf("Keywords: %d | Cities: %d | Total requests: %d",
		len(keywords), len(cities), len(keywords)*len(cities))
	log.Println("==========================================")

	sessionID := newUUID()

	record := Record{
		Date:    time.Now().Format("2006-01-02"),
		Results: []Result{},
	}

	for _, city := range cities {
		log.Printf("── %s ──", city.Name)

		for _, keyword := range keywords {
			count, err := fetchCount(keyword, city, sessionID)
			jobURL := buildJobURL(keyword, city)

			if err != nil {
				log.Printf("  %-15s ERROR: %v", keyword, err)
				record.Results = append(record.Results, Result{
					Keyword: keyword,
					City:    city.Name,
					Count:   -1,
					URL:     jobURL,
				})
			} else {
				log.Printf("  %-15s %d", keyword, count)
				record.Results = append(record.Results, Result{
					Keyword: keyword,
					City:    city.Name,
					Count:   count,
					URL:     jobURL,
				})
			}

			time.Sleep(2 * time.Second)
		}
	}

	if err := appendRecord(record); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Println("==========================================")
	log.Println("Done.")
}
