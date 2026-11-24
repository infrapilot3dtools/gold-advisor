package main

import (
	"encoding/csv"
	"fmt"
	"html"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// EgPost is one parsed price post from the public Telegram web preview (t.me/s/goldprice10000).
type EgPost struct {
	ID                                 int
	Time                               time.Time
	K24, K21, K18, Spread21, Pound, Oz float64
	OzLocal, Bank, Sarf, Parallel      float64
}

const egyptURL = "https://t.me/s/goldprice10000"

var (
	reBlock = regexp.MustCompile(`(?s)<div class="tgme_widget_message_wrap`)
	rePost  = regexp.MustCompile(`data-post="[^/"]+/(\d+)"`)
	reTime  = regexp.MustCompile(`<time datetime="([^"]+)"`)
	reText  = regexp.MustCompile(`(?s)<div class="tgme_widget_message_text[^>]*>(.*?)</div>`)
	reBR    = regexp.MustCompile(`(?i)<br\s*/?>`)
	reTag   = regexp.MustCompile(`<[^>]+>`)
	reNum   = regexp.MustCompile(`[0-9][0-9,]*(?:\.[0-9]+)?`)
)

func asciiDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r == '٬':
			b.WriteRune(',')
		case r == '٫':
			b.WriteRune('.')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// lineVal finds the first line containing key and returns the first number on it.
func lineVal(lines []string, keys ...string) (float64, bool) {
	for _, l := range lines {
		for _, k := range keys {
			if strings.Contains(l, k) {
				idx := strings.Index(l, "←")
				seg := l
				if idx >= 0 {
					seg = l[idx:]
				}
				m := reNum.FindString(seg)
				if m == "" {
					continue
				}
				v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", ""), 64)
				if err == nil {
					return v, true
				}
			}
		}
	}
	return 0, false
}

func parseEgyptPage(page string) ([]EgPost, int) {
	var posts []EgPost
	bad := 0
	for _, blk := range reBlock.Split(page, -1)[1:] {
		pid, tm, tx := rePost.FindStringSubmatch(blk), reTime.FindStringSubmatch(blk), reText.FindStringSubmatch(blk)
		if pid == nil || tm == nil || tx == nil {
			continue
		}
		txt := reBR.ReplaceAllString(tx[1], "\n")
		txt = html.UnescapeString(reTag.ReplaceAllString(txt, ""))
		txt = asciiDigits(txt)
		if !strings.Contains(txt, "عيار 21") || !strings.Contains(txt, "عيار 24") {
			continue // not a price post (news etc.)
		}
		lines := strings.Split(txt, "\n")
		var p EgPost
		p.ID, _ = strconv.Atoi(pid[1])
		t, err := time.Parse(time.RFC3339, tm[1])
		if err != nil {
			bad++
			continue
		}
		p.Time = t
		var ok1, ok2, ok3 bool
		p.K24, ok1 = lineVal(lines, "عيار 24")
		p.K21, ok2 = lineVal(lines, "عيار 21")
		p.K18, ok3 = lineVal(lines, "عيار 18")
		p.Spread21, _ = lineVal(lines, "فرق البيع")
		p.Pound, _ = lineVal(lines, "جنيه ذهب")
		p.Oz, _ = lineVal(lines, "أونصة عالميا", "أونصة عالميًا")
		p.OzLocal, _ = lineVal(lines, "أونصة محليا", "أونصة محليًا")
		p.Bank, _ = lineVal(lines, "البنك")
		p.Sarf, _ = lineVal(lines, "الصاغة")
		p.Parallel, _ = lineVal(lines, "الموازي")
		// sanity checks: karat ratios and local ounce consistency
		if !(ok1 && ok2 && ok3) || p.K24 < 500 || math.Abs(p.K21/p.K24-0.875) > 0.01 || math.Abs(p.K18/p.K24-0.75) > 0.01 {
			bad++
			continue
		}
		if p.OzLocal > 0 && math.Abs(p.OzLocal/(p.K24*31.1035)-1) > 0.02 {
			bad++
			continue
		}
		posts = append(posts, p)
	}
	return posts, bad
}

func (s *Store) loadEgypt() {
	f, err := os.Open(filepath.Join(s.dir, "egypt_posts.csv"))
	if err != nil {
		return
	}
	defer f.Close()
	rows, _ := csv.NewReader(f).ReadAll()
	for i, r := range rows {
		if i == 0 || len(r) < 11 {
			continue
		}
		var p EgPost
		p.ID, _ = strconv.Atoi(r[0])
		p.Time, _ = time.Parse(time.RFC3339, r[1])
		fs := []*float64{&p.K24, &p.K21, &p.K18, &p.Spread21, &p.Pound, &p.Oz, &p.OzLocal, &p.Bank, &p.Sarf, &p.Parallel}
		for k, ptr := range fs {
			*ptr, _ = strconv.ParseFloat(r[2+k], 64)
		}
		s.Egypt = append(s.Egypt, p)
	}
	if n := len(s.Egypt); n > 0 {
		s.Meta["egypt"].Rows = n
	}
}

func (s *Store) saveEgypt() error {
	p := filepath.Join(s.dir, "egypt_posts.csv")
	f, err := os.Create(p + ".tmp")
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	w.Write([]string{"id", "time_utc", "k24", "k21", "k18", "spread21", "gold_pound", "ounce_usd", "ounce_local_egp", "usd_bank", "usd_sagha", "usd_parallel"})
	for _, e := range s.Egypt {
		row := []string{strconv.Itoa(e.ID), e.Time.UTC().Format(time.RFC3339)}
		for _, v := range []float64{e.K24, e.K21, e.K18, e.Spread21, e.Pound, e.Oz, e.OzLocal, e.Bank, e.Sarf, e.Parallel} {
			row = append(row, strconv.FormatFloat(v, 'f', -1, 64))
		}
		w.Write(row)
	}
	w.Flush()
	f.Close()
	return os.Rename(p+".tmp", p)
}

// RefreshEgypt fetches the public preview page once (latest ~20 posts) and keeps new valid price posts.
func (s *Store) RefreshEgypt() error {
	s.fetch.Lock()
	defer s.fetch.Unlock()
	s.mu.Lock()
	s.Meta["egypt"].LastTry = nowStr()
	s.mu.Unlock()
	body, err := getWithBackoff(egyptURL)
	if err != nil {
		return s.fail("egypt", err)
	}
	posts, bad := parseEgyptPage(string(body))
	if len(posts) == 0 {
		return s.fail("egypt", fmt.Errorf("no valid price post found (rejected %d): format may have changed", bad))
	}
	s.mu.Lock()
	have := map[int]bool{}
	for _, e := range s.Egypt {
		have[e.ID] = true
	}
	for _, p := range posts {
		if !have[p.ID] {
			s.Egypt = append(s.Egypt, p)
		}
	}
	if len(s.Egypt) > 20000 {
		s.Egypt = s.Egypt[len(s.Egypt)-20000:]
	}
	s.mu.Unlock()
	if err := s.saveEgypt(); err != nil {
		return s.fail("egypt", err)
	}
	s.mu.Lock()
	m := s.Meta["egypt"]
	m.LastOK, m.LastErr, m.Rows, m.Skipped = nowStr(), "", len(s.Egypt), bad
	s.saveMeta()
	s.mu.Unlock()
	return nil
}

func (s *Store) latestEgypt() (EgPost, bool) {
	var best EgPost
	ok := false
	for _, e := range s.Egypt {
		if !ok || e.Time.After(best.Time) {
			best, ok = e, true
		}
	}
	return best, ok
}
