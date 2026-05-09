package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"time"
)

const (
	baseURL  = "https://www.jobstreet.co.id"
	dataFile = "data/results.jsonl"
)

// Keywords to track
var keywords = []string{
	"sysadmin",
	"devops",
	"cloud",
	"azure",
	"infrastructure",
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

func buildSearchURL(keyword string) string {
	return fmt.Sprintf("%s/jobs?q=%s&l=indonesia", baseURL, url.QueryEscape(keyword))
}

func fetchCount(keyword string) (int, error) {
	searchURL := buildSearchURL(keyword)

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9,en-US;q=0.8")

	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)

	log.Printf("  Status: %d | Body preview: %.300s", resp.StatusCode, string(body))

	if err != nil {
		return 0, fmt.Errorf("read body: %w", err)
	}

	// Parse Next.js JSON script
	if count, ok := extractFromNextData(body); ok {
		return count, nil
	}

	// Regex fallback from HTML
	if count, ok := extractFromHTML(body); ok {
		return count, nil
	}

	return 0, fmt.Errorf("job count not found in response")
}

// Extract count from Next.js
func extractFromNextData(body []byte) (int, bool) {
	re := regexp.MustCompile(`<script id="__NEXT_DATA__" type="application/json">([\s\S]*?)</script>`)
	matches := re.FindSubmatch(body)
	if len(matches) < 2 {
		return 0, false
	}

	var data map[string]interface{}
	if err := json.Unmarshal(matches[1], &data); err != nil {
		return 0, false
	}

	paths := [][]string{
		{"props", "pageProps", "searchResults", "totalCount"},
		{"props", "pageProps", "totalCount"},
		{"props", "pageProps", "jobCount"},
		{"props", "pageProps", "data", "totalCount"},
		{"props", "pageProps", "initialData", "totalCount"},
	}
	for _, path := range paths {
		if count, ok := navigatePath(data, path); ok {
			return count, true
		}
	}

	// find field
	count := recursiveFind(data, []string{"totalCount", "jobCount", "total"})
	return count, count > 0
}

func navigatePath(data map[string]interface{}, path []string) (int, bool) {
	var current interface{} = data
	for i, key := range path {
		m, ok := current.(map[string]interface{})
		if !ok {
			return 0, false
		}
		val, exists := m[key]
		if !exists {
			return 0, false
		}
		if i == len(path)-1 {
			return toInt(val)
		}
		current = val
	}
	return 0, false
}

// find certain keys recursively
func recursiveFind(data interface{}, keys []string) int {
	switch v := data.(type) {
	case map[string]interface{}:
		for _, key := range keys {
			if val, ok := v[key]; ok {
				if count, ok := toInt(val); ok && count > 0 {
					return count
				}
			}
		}
		for _, val := range v {
			if count := recursiveFind(val, keys); count > 0 {
				return count
			}
		}
	case []interface{}:
		for _, item := range v {
			if count := recursiveFind(item, keys); count > 0 {
				return count
			}
		}
	}
	return 0
}

// extractFromHTML
func extractFromHTML(body []byte) (int, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`([\d,\.]+)\s+(?:jobs?|lowongan)`),
		regexp.MustCompile(`(?:found|ditemukan)[^\d]+([\d,\.]+)`),
	}
	stripSep := regexp.MustCompile(`[,\.]`)

	for _, re := range patterns {
		m := re.FindSubmatch(body)
		if len(m) >= 2 {
			cleaned := stripSep.ReplaceAllString(string(m[1]), "")
			if n, err := strconv.Atoi(cleaned); err == nil && n > 0 {
				return n, true
			}
		}
	}
	return 0, false
}

func toInt(v interface{}) (int, bool) {
	stripSep := regexp.MustCompile(`[,\.]`)
	switch val := v.(type) {
	case float64:
		return int(val), true
	case int:
		return val, true
	case string:
		cleaned := stripSep.ReplaceAllString(val, "")
		if n, err := strconv.Atoi(cleaned); err == nil {
			return n, true
		}
	}
	return 0, false
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

	record := Record{
		Date:    time.Now().Format("2006-01-02"),
		Results: []Result{},
	}

	for _, keyword := range keywords {
		searchURL := buildSearchURL(keyword)
		log.Printf("Scraping: %-15s -> %s", keyword, searchURL)

		count, err := fetchCount(keyword)
		if err != nil {
			log.Printf("  ERROR: %v", err)
			record.Results = append(record.Results, Result{
				Keyword: keyword,
				Count:   -1,
				URL:     searchURL,
			})
		} else {
			log.Printf("  Found: %d jobs", count)
			record.Results = append(record.Results, Result{
				Keyword: keyword,
				Count:   count,
				URL:     searchURL,
			})
		}

		// Avoid rate limiting with a short delay between requests
		time.Sleep(3 * time.Second)
	}

	if err := appendRecord(record); err != nil {
		log.Fatalf("Failed to save: %v", err)
	}

	log.Println("==========================================")
	log.Println("Done. Data appended to", dataFile)
}
