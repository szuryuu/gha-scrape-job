# JobStreet Job Tracker

A GitHub Action, written in Go, that scrapes weekly job counts from JobStreet Indonesia for selected keywords and stores the results as a time-series dataset.

---

## What it does

- Fetches job counts from [JobStreet Indonesia](https://www.jobstreet.co.id) for a configurable list of keywords
- Appends one JSON record per run to `data/results.jsonl`
- Runs automatically every Monday at 08:00 WIB
- Commits the updated dataset back to the repository
- The dataset is publicly accessible via raw GitHub URL — no server required

---

## Data format

Results are stored as [JSON Lines](https://jsonlines.org/) — one record per line, one line per run:

```jsonl
{"date":"2026-05-12","results":[{"keyword":"devops","count":142,"url":"https://..."},{"keyword":"cloud","count":891,"url":"https://..."}]}
{"date":"2026-05-19","results":[{"keyword":"devops","count":155,"url":"https://..."},{"keyword":"cloud","count":902,"url":"https://..."}]}
```

A count of `-1` means the request failed for that keyword (e.g. bot detection, timeout).

---

## Accessing the data

Because the repository is public, `results.jsonl` is accessible as a free API endpoint:

```
https://raw.githubusercontent.com/<your-username>/gha-scrape-job/main/data/results.jsonl
```

Fetch and parse it from any frontend:

```js
const res = await fetch(
  "https://raw.githubusercontent.com/..../data/results.jsonl",
);
const text = await res.text();
const records = text.trim().split("\n").map(JSON.parse);
```

---

## Setup

### 1. Fork or use this repository as a template

Click **"Use this template"** at the top of the page and create a new repository.

### 2. Make the repository public

The raw GitHub URL is only accessible if the repo is public. Job count data from JobStreet is public information, so there is nothing sensitive here.

### 3. Enable Actions write permission

Go to **Settings → Actions → General → Workflow permissions** and select **"Read and write permissions"**.

### 4. Configure keywords

Open `main.go` and edit the `keywords` slice:

```go
var keywords = []string{
    "devops",
    "cloud",
    "sysadmin",
    "azure",
    "infrastructure",
}
```

Commit and push. The workflow will use the updated list on its next run.

### 5. Run manually (optional)

Go to **Actions → JobStreet Job Tracker → Run workflow** to trigger a run immediately.

---

## Limitations

| Issue                  | Detail                                                                                                                                 |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Bot detection          | JobStreet may block requests from GitHub Actions IP ranges (datacenter IPs). If all counts return `-1`, this is the likely cause.      |
| Site structure changes | The scraper parses `__NEXT_DATA__` from the page HTML. If JobStreet changes its frontend framework, the parser may need to be updated. |
| Rate limiting          | A 3-second delay between keyword requests reduces the chance of rate limiting.                                                         |
| Weekly cadence         | The cron runs once per week. For higher-frequency data, adjust the cron schedule — but be respectful of the site's resources.          |

### Debugging -1 results

Add this line to `fetchCount` in `main.go` after `io.ReadAll` to inspect what the server actually returns:

```go
log.Printf("DEBUG status=%d body=%.500s", resp.StatusCode, string(body))
```

Run the workflow manually and check the Actions log. The response body will show whether the request was blocked, redirected, or returned unexpected HTML.

---

## File structure

```
.
├── .github/
│   └── workflows/
│       └── scrape.yml     # GitHub Actions workflow (weekly cron)
├── data/
│   └── results.jsonl      # Time-series dataset (append-only)
├── main.go                # Scraper logic
├── go.mod
└── README.md
```

---

## License

MIT
