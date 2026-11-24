package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

type Bar struct {
	Date       string  `json:"date"`
	O, H, L, C float64 `json:"-"`
}

type Obs struct {
	Date string
	V    float64
}

type SourceMeta struct {
	LastOK   string `json:"lastOK"`
	LastTry  string `json:"lastTry"`
	LastErr  string `json:"lastErr"`
	Rows     int    `json:"rows"`
	Repaired int    `json:"repaired"`
	Skipped  int    `json:"skipped"`
}

type Store struct {
	mu    sync.Mutex
	dir   string
	Gold  []Bar
	Real  []Obs
	USD   []Obs
	Egypt []EgPost
	Meta  map[string]*SourceMeta
	fetch sync.Mutex
}

var nyLoc *time.Location

func init() {
	l, err := time.LoadLocation("America/New_York")
	if err != nil {
		l = time.FixedZone("EST", -5*3600)
	}
	nyLoc = l
}

func NewStore(dir string) *Store {
	s := &Store{dir: dir, Meta: map[string]*SourceMeta{"gold": {}, "real": {}, "usd": {}, "egypt": {}}}
	os.MkdirAll(dir, 0o755)
	s.loadMeta()
	s.Gold = readBars(filepath.Join(dir, "gold_gcf_daily.csv"))
	s.Real = readObs(filepath.Join(dir, "fred_DFII10.csv"))
	s.USD = readObs(filepath.Join(dir, "fred_DTWEXBGS.csv"))
	s.loadEgypt()
	for k, m := range map[string]int{"gold": len(s.Gold), "real": len(s.Real), "usd": len(s.USD)} {
		s.Meta[k].Rows = m
	}
	return s
}

func (s *Store) loadMeta() {
	b, err := os.ReadFile(filepath.Join(s.dir, "meta.json"))
	if err != nil {
		return
	}
	m := map[string]*SourceMeta{}
	if json.Unmarshal(b, &m) == nil {
		for k, v := range m {
			s.Meta[k] = v
		}
	}
}

func (s *Store) saveMeta() {
	b, _ := json.MarshalIndent(s.Meta, "", " ")
	os.WriteFile(filepath.Join(s.dir, "meta.json"), b, 0o644)
}

func readBars(p string) []Bar {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, _ := r.ReadAll()
	var out []Bar
	for i, row := range rows {
		if i == 0 || len(row) < 5 {
			continue
		}
		o, _ := strconv.ParseFloat(row[1], 64)
		h, _ := strconv.ParseFloat(row[2], 64)
		l, _ := strconv.ParseFloat(row[3], 64)
		c, _ := strconv.ParseFloat(row[4], 64)
		out = append(out, Bar{row[0], o, h, l, c})
	}
	return out
}

func writeBars(p string, bars []Bar) error {
	tmp := p + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	w.Write([]string{"date", "open", "high", "low", "close"})
	for _, b := range bars {
		w.Write([]string{b.Date, fmt.Sprintf("%.4f", b.O), fmt.Sprintf("%.4f", b.H), fmt.Sprintf("%.4f", b.L), fmt.Sprintf("%.4f", b.C)})
	}
	w.Flush()
	f.Close()
	return os.Rename(tmp, p)
}

func readObs(p string) []Obs {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	rows, _ := csv.NewReader(f).ReadAll()
	var out []Obs
	for i, row := range rows {
		if i == 0 || len(row) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(row[1], 64)
		if err != nil {
			continue
		}
		out = append(out, Obs{row[0], v})
	}
	return out
}

func writeObs(p, name string, obs []Obs) error {
	tmp := p + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	w.Write([]string{"date", name})
	for _, o := range obs {
		w.Write([]string{o.Date, strconv.FormatFloat(o.V, 'f', -1, 64)})
	}
	w.Flush()
	f.Close()
	return os.Rename(tmp, p)
}

func httpGet(url string, timeout time.Duration) ([]byte, int, error) {
	req, _ := http.NewRequest("GET", url, nil)
	// FRED resets connections that carry a browser User-Agent (tested), so it gets the Go default; Yahoo and Telegram need a browser one.
	if !strings.Contains(url, "fred.stlouisfed.org") {
		req.Header.Set("User-Agent", userAgent)
	}
	req.Header.Set("Accept", "*/*")
	c := &http.Client{Timeout: timeout}
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	return b, resp.StatusCode, err
}

// getWithBackoff retries on 429 / 5xx / network errors (max 3 tries, 20s then 60s pause).
func getWithBackoff(url string) ([]byte, error) {
	waits := []time.Duration{0, 15 * time.Second, 45 * time.Second}
	var last error
	for i, w := range waits {
		if w > 0 {
			time.Sleep(w)
		}
		b, code, err := httpGet(url, 60*time.Second)
		if err == nil && code == 200 {
			return b, nil
		}
		if err != nil {
			last = err
		} else {
			last = fmt.Errorf("HTTP %d", code)
		}
		if err == nil && code != 429 && code < 500 {
			break
		}
		_ = i
	}
	return nil, last
}

type yahooResp struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open  []*float64 `json:"open"`
					High  []*float64 `json:"high"`
					Low   []*float64 `json:"low"`
					Close []*float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code, Description string
		} `json:"error"`
	} `json:"chart"`
}

func (s *Store) RefreshGold() error {
	s.fetch.Lock()
	defer s.fetch.Unlock()
	s.mu.Lock()
	m := s.Meta["gold"]
	m.LastTry = nowStr()
	existing := append([]Bar(nil), s.Gold...)
	s.mu.Unlock()

	// Must use period1/period2: range=max returns only monthly-like bars.
	p1 := int64(946684800)
	if len(existing) > 1500 {
		t, err := time.Parse("2006-01-02", existing[len(existing)-1].Date)
		if err == nil {
			p1 = t.AddDate(0, 0, -20).Unix()
		}
	}
	p2 := time.Now().Add(48 * time.Hour).Unix()
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/GC=F?period1=%d&period2=%d&interval=1d&events=history", p1, p2)
	body, err := getWithBackoff(url)
	if err != nil {
		return s.fail("gold", err)
	}
	var yr yahooResp
	if err := json.Unmarshal(body, &yr); err != nil {
		return s.fail("gold", fmt.Errorf("JSON: %v", err))
	}
	if len(yr.Chart.Result) == 0 || len(yr.Chart.Result[0].Indicators.Quote) == 0 {
		return s.fail("gold", fmt.Errorf("no result"))
	}
	r := yr.Chart.Result[0]
	q := r.Indicators.Quote[0]
	byDate := map[string]Bar{}
	for _, b := range existing {
		byDate[b.Date] = b
	}
	repaired, skipped, added := 0, 0, 0
	for i, ts := range r.Timestamp {
		if i >= len(q.Open) || i >= len(q.High) || i >= len(q.Low) || i >= len(q.Close) {
			break
		}
		if q.Open[i] == nil || q.High[i] == nil || q.Low[i] == nil || q.Close[i] == nil {
			skipped++
			continue
		}
		o, h, l, c := *q.Open[i], *q.High[i], *q.Low[i], *q.Close[i]
		if o <= 0 || h <= 0 || l <= 0 || c <= 0 || math.IsNaN(o+h+l+c) {
			skipped++
			continue
		}
		hh := math.Max(math.Max(o, c), math.Max(h, l))
		ll := math.Min(math.Min(o, c), math.Min(h, l))
		if hh != h || ll != l {
			repaired++
			h, l = hh, ll
		}
		d := time.Unix(ts, 0).In(nyLoc).Format("2006-01-02")
		if _, ok := byDate[d]; !ok {
			added++
		}
		byDate[d] = Bar{d, o, h, l, c}
	}
	if len(existing) == 0 && len(byDate) < 3000 {
		return s.fail("gold", fmt.Errorf("history too short (%d bars)", len(byDate)))
	}
	if len(byDate) == 0 {
		return s.fail("gold", fmt.Errorf("no valid bars"))
	}
	all := make([]Bar, 0, len(byDate))
	for _, b := range byDate {
		all = append(all, b)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Date < all[j].Date })
	if err := writeBars(filepath.Join(s.dir, "gold_gcf_daily.csv"), all); err != nil {
		return s.fail("gold", err)
	}
	s.mu.Lock()
	s.Gold = all
	m.LastOK, m.LastErr, m.Rows = nowStr(), "", len(all)
	if len(existing) == 0 || p1 == 946684800 {
		m.Repaired, m.Skipped = repaired, skipped
	}
	s.saveMeta()
	s.mu.Unlock()
	return nil
}

func (s *Store) RefreshFred(key, id string, dst *[]Obs, file string) error {
	s.fetch.Lock()
	defer s.fetch.Unlock()
	s.mu.Lock()
	s.Meta[key].LastTry = nowStr()
	s.mu.Unlock()
	body, err := getWithBackoff("https://fred.stlouisfed.org/graph/fredgraph.csv?id=" + id)
	if err != nil {
		return s.fail(key, err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil || len(rows) < 2 || len(rows[0]) < 2 || !strings.Contains(rows[0][1], id) {
		return s.fail(key, fmt.Errorf("unexpected FRED format"))
	}
	var obs []Obs
	skipped := 0
	for _, row := range rows[1:] {
		v, e := strconv.ParseFloat(row[1], 64)
		if e != nil {
			skipped++
			continue
		}
		obs = append(obs, Obs{row[0], v})
	}
	if len(obs) < 500 {
		return s.fail(key, fmt.Errorf("too few rows (%d)", len(obs)))
	}
	if err := writeObs(filepath.Join(s.dir, file), id, obs); err != nil {
		return s.fail(key, err)
	}
	s.mu.Lock()
	*dst = obs
	m := s.Meta[key]
	m.LastOK, m.LastErr, m.Rows, m.Skipped = nowStr(), "", len(obs), skipped
	s.saveMeta()
	s.mu.Unlock()
	return nil
}

func (s *Store) fail(key string, err error) error {
	s.mu.Lock()
	s.Meta[key].LastErr = err.Error()
	s.saveMeta()
	s.mu.Unlock()
	return err
}

func nowStr() string { return time.Now().Format("2006-01-02 15:04:05") }

func parseLocal(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	return t, err == nil
}
