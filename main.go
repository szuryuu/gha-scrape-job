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

// Query only total count
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
	Count   int    `json:"count"`
	URL     string `json:"url"`
}

type Record struct {
	Date    string   `json:"date"`
	Results []Result `json:"results"`
}

// Generate random UUID v4
func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func buildJobURL(keyword string) string {
	return fmt.Sprintf("https://id.jobstreet.com/id/%s-jobs", keyword)
}

func fetchCount(keyword, sessionID string) (int, error) {
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

	// Set headers to mimic a real browser request
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
	log.Println("==========================================")

	// Generate a new session ID for this run
	sessionID := newUUID()
	log.Printf("Session: %s", sessionID)

	record := Record{
		Date:    time.Now().Format("2006-01-02"),
		Results: []Result{},
	}

	for _, keyword := range keywords {
		log.Printf("Fetching: %s", keyword)

		count, err := fetchCount(keyword, sessionID)
		if err != nil {
			log.Printf("  ERROR: %v", err)
			record.Results = append(record.Results, Result{
				Keyword: keyword,
				Count:   -1,
				URL:     buildJobURL(keyword),
			})
		} else {
			log.Printf("  Count: %d", count)
			record.Results = append(record.Results, Result{
				Keyword: keyword,
				Count:   count,
				URL:     buildJobURL(keyword),
			})
		}

		time.Sleep(3 * time.Second)
	}

	if err := appendRecord(record); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Println("==========================================")
	log.Println("Done.")
}
