package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Settings is the subset of the PC app's settings that buildState reads.
type Settings struct {
	Listen       string  `json:"listen"`
	SaudiMode    string  `json:"saudiMode"`
	EgyptMode    string  `json:"egyptMode"`
	EgyptPrice21 float64 `json:"egyptPrice21"`
	EgyptSource  string  `json:"egyptSource"`
	EgyptAsOf    string  `json:"egyptAsOf"`
}

type published struct {
	GeneratedAt string `json:"generatedAt"`
	State       State  `json:"state"`
	Refreshing  bool   `json:"refreshing"`
}

func fail(msg string, err error) {
	fmt.Fprintln(os.Stderr, "ERROR:", msg, err)
	os.Exit(1)
}

func loadPrev(url string) *published {
	if url == "" {
		return nil
	}
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Get(url + "?_=" + fmt.Sprint(time.Now().Unix()))
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	var p published
	if json.Unmarshal(b, &p) != nil {
		return nil
	}
	return &p
}

func main() {
	out := flag.String("out", "data.json", "output file")
	prevURL := flag.String("prev", "", "URL of the previously published data.json (used only to keep the last Egypt price if Telegram fails)")
	flag.Parse()
	dir, _ := os.MkdirTemp("", "gold")
	defer os.RemoveAll(dir)
	store := NewStore(dir)

	// Core sources: any failure fails the run, so the last good site stays online and the page shows its age.
	if err := store.RefreshGold(); err != nil {
		fail("gold", err)
	}
	time.Sleep(2 * time.Second)
	if err := store.RefreshFred("real", "DFII10", &store.Real, "fred_DFII10.csv"); err != nil {
		fail("DFII10", err)
	}
	if err := store.RefreshFred("usd", "DTWEXBGS", &store.USD, "fred_DTWEXBGS.csv"); err != nil {
		fail("DTWEXBGS", err)
	}
	egErr := store.RefreshEgypt()
	if egErr != nil {
		fmt.Fprintln(os.Stderr, "WARNING: egypt:", egErr)
	}

	cfg := Settings{SaudiMode: "futures_estimate", EgyptMode: "telegram"}
	st := buildState(store, cfg)

	// Egypt carry-over: keep the last published Egypt price (it will show as old, the page checks its time).
	if st.Local.EgyptStat == "unavailable" || st.Local.EgyptStat == "" {
		if prev := loadPrev(*prevURL); prev != nil && prev.State.Local.EgyptK != nil && prev.State.Local.EgyptTime != "" {
			pl := prev.State.Local
			st.Local.EgyptK, st.Local.EgyptText, st.Local.EgyptTime = pl.EgyptK, pl.EgyptText, pl.EgyptTime
			st.Local.EgyptStat = "stale"
			st.Local.EgyptNote = "تعذر تحديث سعر مصر في آخر مرة، فده آخر سعر اتسجل. " + pl.EgyptNote
		}
	}

	// Publish derived numbers only: no raw third-party text, no price history files.
	st.Settings = Settings{}
	for i := range st.Health {
		st.Health[i].LastErr = ""
		st.Health[i].Retrieved = ""
	}
	for _, t := range st.Paper.Trades {
		delete(t, "entryPx")
		delete(t, "exitPx")
	}
	pub := published{GeneratedAt: time.Now().UTC().Format(time.RFC3339), State: st}
	b, _ := json.MarshalIndent(pub, "", " ")
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fail("write", err)
	}
	fmt.Println("wrote", *out, len(b), "bytes; egypt:", st.Local.EgyptStat)
}
