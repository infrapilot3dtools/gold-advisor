package main

import (
	"fmt"
	"math"
	"time"
)

type Health struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	Source     string `json:"source"`
	LastObs    string `json:"lastObs"`
	AgeDays    int    `json:"ageDays"`
	Retrieved  string `json:"retrieved"`
	Expected   string `json:"expected"`
	MaxAgeDays int    `json:"maxAgeDays"`
	Status     string `json:"status"` // ok | stale | unavailable
	Rows       int    `json:"rows"`
	Note       string `json:"note"`
	LastErr    string `json:"lastErr"`
}

type Group struct {
	Name   string   `json:"name"`
	Score  *float64 `json:"score"`
	Items  []string `json:"items"`
	Status string   `json:"status"`
}

type Horizon struct {
	Name    string   `json:"name"`
	GoldRet float64  `json:"goldRet"`
	RealChg *float64 `json:"realChg"`
	USDChg  *float64 `json:"usdChg"`
	Support string   `json:"support"`
}

type Trade struct {
	Entry, Exit     string
	EntryPx, ExitPx float64
	RetPct          float64
	Reason          string
}

type Paper struct {
	State      string           `json:"state"`
	StateText  string           `json:"stateText"`
	Details    []string         `json:"details"`
	Gates      []string         `json:"gates"`
	EntryOK    bool             `json:"entryOK"`
	Trades     []map[string]any `json:"trades"`
	TradeCount int              `json:"tradeCount"`
	Params     string           `json:"params"`
}

type Direction struct {
	Label  string   `json:"label"`
	Score  *float64 `json:"score"`
	Groups []Group  `json:"groups"`
	Notes  []string `json:"notes"`
}

type Local struct {
	SaudiMode string             `json:"saudiMode"`
	Saudi     map[string]float64 `json:"saudi"`
	SaudiNote string             `json:"saudiNote"`
	EgyptText string             `json:"egyptText"`
	EgyptVal  float64            `json:"egyptVal"`
	EgyptNote string             `json:"egyptNote"`
	EgyptMode string             `json:"egyptMode"`
	EgyptK    map[string]float64 `json:"egyptK"`
	EgyptStat string             `json:"egyptStat"`
	EgyptTime string             `json:"egyptTime"`
}

type State struct {
	Now       string         `json:"now"`
	Price     map[string]any `json:"price"`
	Direction Direction      `json:"direction"`
	Horizons  []Horizon      `json:"horizons"`
	Health    []Health       `json:"health"`
	Paper     Paper          `json:"paper"`
	Local     Local          `json:"local"`
	Settings  Settings       `json:"settings"`
}

var egLoc = func() *time.Location {
	l, err := time.LoadLocation("Africa/Cairo")
	if err != nil {
		return time.FixedZone("EG", 2*3600)
	}
	return l
}()

func sma(b []Bar, i, n int) float64 {
	s := 0.0
	for k := i - n + 1; k <= i; k++ {
		s += b[k].C
	}
	return s / float64(n)
}

func atr(b []Bar, i, n int) float64 {
	s := 0.0
	for k := i - n + 1; k <= i; k++ {
		tr := b[k].H - b[k].L
		if v := math.Abs(b[k].H - b[k-1].C); v > tr {
			tr = v
		}
		if v := math.Abs(b[k].L - b[k-1].C); v > tr {
			tr = v
		}
		s += tr
	}
	return s / float64(n)
}

func hiPrev(b []Bar, i, n int) float64 {
	m := 0.0
	for k := i - n; k < i; k++ {
		if b[k].H > m {
			m = b[k].H
		}
	}
	return m
}

// completed returns bars strictly before today's New York date (today's bar may be partial).
func completed(b []Bar) ([]Bar, bool) {
	today := time.Now().In(nyLoc).Format("2006-01-02")
	n := len(b)
	has := false
	for n > 0 && b[n-1].Date >= today {
		n--
		has = true
	}
	return b[:n], has
}

func daysSince(date string) int {
	t, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		return 9999
	}
	now := time.Now().In(nyLoc)
	nd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return int(nd.Sub(t).Hours() / 24)
}

func fptr(v float64) *float64 { return &v }

func buildHealth(s *Store) []Health {
	mk := func(key, name, src, expected string, maxAge int, lastObs string, rows int, note string) Health {
		h := Health{Key: key, Name: name, Source: src, Expected: expected, MaxAgeDays: maxAge, Rows: rows, Note: note}
		m := s.Meta[key]
		h.Retrieved, h.LastErr = m.LastOK, m.LastErr
		if lastObs == "" {
			h.Status, h.LastObs, h.AgeDays = "unavailable", "-", -1
			return h
		}
		h.LastObs = lastObs
		h.AgeDays = daysSince(lastObs)
		if h.AgeDays > maxAge {
			h.Status = "stale"
		} else {
			h.Status = "ok"
		}
		return h
	}
	var g, r, u string
	if n := len(s.Gold); n > 0 {
		g = s.Gold[n-1].Date
	}
	if n := len(s.Real); n > 0 {
		r = s.Real[n-1].Date
	}
	if n := len(s.USD); n > 0 {
		u = s.USD[n-1].Date
	}
	hl := []Health{
		mk("gold", "الذهب (عقود COMEX الآجلة GC=F، دولار/أونصة)", "Yahoo Finance (غير رسمي)", "يوم تداول جديد كل يوم عمل أمريكي", 5, g, len(s.Gold), "عقود آجلة وليست سبوت. حد القِدم 5 أيام (مقترح، غير مُتحقق منه)."),
		mk("real", "العائد الحقيقي 10 سنوات (DFII10)", "FRED (CSV عام)", "قراءة يومية تظهر عادة بعد يوم عمل أو أكثر", 6, r, len(s.Real), "حد القِدم 6 أيام (مقترح، غير مُتحقق منه)."),
		mk("usd", "مؤشر الدولار الواسع (DTWEXBGS)", "FRED (CSV عام)", "يتحدث دوريا بتأخر قد يصل أسبوعا", 10, u, len(s.USD), "ليس DXY. حد القِدم 10 أيام (مقترح، غير مُتحقق منه)."),
	}
	if e, ok := s.latestEgypt(); ok || s.Meta["egypt"].LastTry != "" {
		h := Health{Key: "egypt", Name: "سعر الذهب في مصر (قناة تيليجرام goldprice10000)", Source: "t.me/s/goldprice10000 (غير رسمي)", Expected: "منشور كل 15 دقيقة تقريبا في ساعات التداول", MaxAgeDays: 4, Rows: len(s.Egypt), Note: "إرشادي. حقوق الاستخدام غير مؤكدة. الشكل قابل للتغيير بدون إنذار.", Retrieved: s.Meta["egypt"].LastOK, LastErr: s.Meta["egypt"].LastErr, Status: "unavailable", LastObs: "-", AgeDays: -1}
		if ok {
			h.LastObs = e.Time.In(egLoc).Format("2006-01-02")
			h.AgeDays = int(time.Since(e.Time).Hours() / 24)
			if time.Since(e.Time) <= 18*time.Hour {
				h.Status = "ok"
			} else if time.Since(e.Time) <= 96*time.Hour {
				h.Status = "stale"
			}
		}
		hl = append(hl, h)
	}
	return hl
}

func chg(o []Obs, n int) *float64 {
	if len(o) <= n {
		return nil
	}
	return fptr(o[len(o)-1].V - o[len(o)-1-n].V)
}

func pchg(o []Obs, n int) *float64 {
	if len(o) <= n || o[len(o)-1-n].V == 0 {
		return nil
	}
	return fptr((o[len(o)-1].V/o[len(o)-1-n].V - 1) * 100)
}

func labelOf(sc float64) string {
	switch {
	case sc >= 0.25:
		return "ظروف داعمة للذهب"
	case sc <= -0.25:
		return "ظروف معاكسة للذهب"
	}
	return "محايد / متضارب"
}

func buildState(s *Store, cfg Settings) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := State{Now: nowStr(), Settings: cfg}
	st.Health = buildHealth(s)
	hs := map[string]Health{}
	for _, h := range st.Health {
		hs[h.Key] = h
	}
	bars, partial := completed(s.Gold)
	n := len(bars)

	// price
	st.Price = map[string]any{"available": false}
	if len(s.Gold) > 0 {
		lb := s.Gold[len(s.Gold)-1]
		prev := 0.0
		if len(s.Gold) > 1 {
			prev = s.Gold[len(s.Gold)-2].C
		}
		st.Price = map[string]any{"available": true, "date": lb.Date, "close": lb.C, "prev": prev, "partial": partial, "label": "العقود الآجلة (COMEX GC=F) بالدولار للأونصة، وليس السبوت"}
	}

	// ---- direction ----
	var trend, macro Group
	trend = Group{Name: "اتجاه سعر الذهب", Status: "unavailable"}
	if n >= 253 && hs["gold"].Status != "unavailable" {
		i := n - 1
		c := bars[i].C
		s50, s200 := sma(bars, i, 50), sma(bars, i, 200)
		r63 := c/bars[i-63].C - 1
		sg := func(v bool) float64 {
			if v {
				return 1
			}
			return -1
		}
		tot := sg(c > s200) + sg(c > s50) + sg(r63 > 0)
		trend.Score = fptr(tot / 3)
		trend.Status = hs["gold"].Status
		trend.Items = []string{
			fmt.Sprintf("الإغلاق %.1f مقابل متوسط 200 يوم %.1f: %s", c, s200, map[bool]string{true: "أعلى (+1)", false: "أدنى (-1)"}[c > s200]),
			fmt.Sprintf("الإغلاق مقابل متوسط 50 يوم %.1f: %s", s50, map[bool]string{true: "أعلى (+1)", false: "أدنى (-1)"}[c > s50]),
			fmt.Sprintf("التغير خلال 63 يوم تداول: %+.1f%% (%s)", r63*100, map[bool]string{true: "+1", false: "-1"}[r63 > 0]),
		}
	}
	macro = Group{Name: "الاقتصاد الكلي (العائد الحقيقي + الدولار معا)", Status: "unavailable"}
	rc, uc := chg(s.Real, 20), pchg(s.USD, 20)
	if rc != nil && uc != nil && hs["real"].Status == "ok" && hs["usd"].Status == "ok" {
		a, b := 0.0, 0.0
		if *rc < -0.05 {
			a = 1
		} else if *rc > 0.05 {
			a = -1
		}
		if *uc < -0.5 {
			b = 1
		} else if *uc > 0.5 {
			b = -1
		}
		macro.Score = fptr((a + b) / 2)
		macro.Status = "ok"
		macro.Items = []string{
			fmt.Sprintf("العائد الحقيقي تغير %+.2f نقطة خلال 20 قراءة (انخفاضه داعم للذهب): %+.0f", *rc, a),
			fmt.Sprintf("الدولار الواسع تغير %+.2f%% خلال 20 قراءة (انخفاضه داعم للذهب): %+.0f", *uc, b),
		}
	} else if hs["real"].Status != "ok" || hs["usd"].Status != "ok" {
		macro.Status = "stale"
		macro.Items = []string{"بيانات الاقتصاد الكلي غير متاحة أو متأخرة، فلا تدخل في التقييم."}
	}
	dir := Direction{Groups: []Group{trend, macro}}
	switch {
	case trend.Score != nil && macro.Score != nil:
		sc := (*trend.Score + *macro.Score) / 2
		dir.Score, dir.Label = fptr(sc), labelOf(sc)
		if *trend.Score*(*macro.Score) < 0 {
			dir.Notes = append(dir.Notes, "المجموعتان تعطيان إشارتين متعاكستين: خذ التقييم بحذر.")
		}
	case trend.Score != nil:
		dir.Score, dir.Label = trend.Score, labelOf(*trend.Score)+" (من الاتجاه فقط)"
		dir.Notes = append(dir.Notes, "مجموعة الاقتصاد الكلي غير متاحة، فالتقييم مبني على اتجاه السعر فقط.")
	default:
		dir.Label = "غير متاح"
		dir.Notes = append(dir.Notes, "بيانات الذهب غير كافية.")
	}
	dir.Notes = append(dir.Notes,
		"هذا وصف لظروف السوق وليس توقعا ولا احتمال ربح. القراءة السلبية تعني ظروفا معاكسة فقط، وليست توصية ببيع على المكشوف.",
		"الأوزان والعتبات المستخدمة ثابتة ومقترحة ولم تُختبر أو تُعايَر.")
	st.Direction = dir

	// ---- horizons ----
	for _, h := range []struct {
		name string
		d    int
	}{{"أسابيع (~شهر)", 21}, {"شهور (~3 أشهر)", 63}, {"سنوات (~سنة)", 252}} {
		hz := Horizon{Name: h.name}
		if n > h.d {
			hz.GoldRet = (bars[n-1].C/bars[n-1-h.d].C - 1) * 100
		}
		hz.RealChg, hz.USDChg = chg(s.Real, h.d), pchg(s.USD, h.d)
		sup, tot := 0, 0
		if n > h.d {
			tot++
			if hz.GoldRet > 0 {
				sup++
			}
		}
		if hz.RealChg != nil && hs["real"].Status == "ok" {
			tot++
			if *hz.RealChg < 0 {
				sup++
			}
		}
		if hz.USDChg != nil && hs["usd"].Status == "ok" {
			tot++
			if *hz.USDChg < 0 {
				sup++
			}
		}
		hz.Support = fmt.Sprintf("%d من %d عوامل داعمة", sup, tot)
		st.Horizons = append(st.Horizons, hz)
	}

	// ---- paper model ----
	st.Paper = paperModel(bars, partial, hs)

	// ---- local prices ----
	st.Local = Local{SaudiMode: cfg.SaudiMode, Saudi: map[string]float64{}}
	if cfg.SaudiMode == "futures_estimate" && len(s.Gold) > 0 {
		c := s.Gold[len(s.Gold)-1].C
		g24 := c / 31.1035 * 3.75
		st.Local.Saudi = map[string]float64{"24": g24, "22": g24 * 22 / 24, "21": g24 * 0.875, "18": g24 * 0.75}
		st.Local.SaudiNote = "تقدير من سعر العقود الآجلة وليس السبوت: (دولار/أونصة ÷ 31.1035 × 3.75). لا يشمل العلاوة أو الضريبة أو المصنعية، وليس سعرا قابلا للتنفيذ لدى تاجر."
	}
	st.Local.EgyptMode = cfg.EgyptMode
	st.Local.EgyptText = "غير متاح"
	st.Local.EgyptNote = "لا يوجد سعر لمصر حاليا. يمكنك إدخال لقطة يدوية من الإعدادات."
	st.Local.EgyptStat = "unavailable"
	if cfg.EgyptMode == "telegram" {
		if e, ok := s.latestEgypt(); ok {
			age := time.Since(e.Time)
			st.Local.EgyptTime = e.Time.UTC().Format(time.RFC3339)
			loc := e.Time.In(egLoc)
			switch {
			case age <= 18*time.Hour:
				st.Local.EgyptStat = "ok"
			case age <= 96*time.Hour:
				st.Local.EgyptStat = "stale"
			}
			if st.Local.EgyptStat != "unavailable" {
				st.Local.EgyptK = map[string]float64{"24": e.K24, "21": e.K21, "18": e.K18, "spread21": e.Spread21, "pound": e.Pound, "bank": e.Bank, "sagha": e.Sarf, "parallel": e.Parallel}
				st.Local.EgyptText = fmt.Sprintf("عيار 21: %.0f جنيه/جرام", e.K21)
				st.Local.EgyptNote = fmt.Sprintf("آخر منشور في قناة تيليجرام عامة (goldprice10000) بتاريخ %s (توقيت القاهرة، %d ساعة مضت). غير شامل المصنعية. القناة تنشر سعرا واحدا لكل عيار بدون توضيح هل هو بيع أم شراء، مع ذكر فرق البيع والشراء لعيار 21. مصدر غير رسمي وحقوق استخدامه غير مؤكدة.", loc.Format("2006-01-02 15:04"), int(age.Hours()))
				if st.Local.EgyptStat == "stale" {
					st.Local.EgyptNote = "تنبيه: السعر قديم. " + st.Local.EgyptNote
				}
			}
		}
		if st.Local.EgyptStat == "unavailable" {
			st.Local.EgyptNote = "تعذر قراءة سعر حديث من قناة تيليجرام (لم يصل منشور صالح خلال 4 أيام أو فشل الجلب). " + st.Local.EgyptNote
		}
	}
	if st.Local.EgyptStat == "unavailable" && (cfg.EgyptMode == "manual" || cfg.EgyptMode == "telegram") && cfg.EgyptPrice21 > 0 {
		st.Local.EgyptVal = cfg.EgyptPrice21
		st.Local.EgyptText = fmt.Sprintf("%.0f جنيه/جرام عيار 21 (إدخال يدوي)", cfg.EgyptPrice21)
		st.Local.EgyptNote = fmt.Sprintf("إدخال يدوي بتاريخ %s. المصدر: %s. لقطة إرشادية وليست سعرا مضمونا.", cfg.EgyptAsOf, cfg.EgyptSource)
		st.Local.EgyptStat = "manual"
	}
	return st
}

func paperModel(b []Bar, partial bool, hs map[string]Health) Paper {
	p := Paper{Params: "SMA50، اختراق أعلى سعر لآخر 20 جلسة مكتملة، ATR20، وقف أولي 2×ATR، مركز شراء واحد بلا رافعة، الخروج بالوقف أو الإغلاق تحت SMA50. كل الأرقام تجريبية ولم تُختبر."}
	n := len(b)
	if n < 260 {
		p.State, p.StateText = "nodata", "لا توجد بيانات كافية"
		return p
	}
	pos, pending, pendingExit := false, false, false
	var entryPx, stop float64
	var entryDate string
	var sigATR float64
	var trades []Trade
	for i := 51; i < n; i++ {
		if pendingExit {
			trades = append(trades, Trade{entryDate, b[i].Date, entryPx, b[i].O, (b[i].O/entryPx - 1) * 100, "إغلاق تحت SMA50"})
			pos, pendingExit = false, false
		}
		if pending {
			entryPx, entryDate = b[i].O, b[i].Date
			stop = entryPx - 2*sigATR
			pos, pending = true, false
		}
		if pos {
			if b[i].L <= stop {
				px := stop
				if b[i].O < stop {
					px = b[i].O
				}
				trades = append(trades, Trade{entryDate, b[i].Date, entryPx, px, (px/entryPx - 1) * 100, "ضرب الوقف"})
				pos = false
			} else if b[i].C < sma(b, i, 50) {
				pendingExit = true
			}
		}
		if !pos && !pending {
			if b[i].C > sma(b, i, 50) && b[i].C > hiPrev(b, i, 20) {
				pending = true
				sigATR = atr(b, i, 20)
			}
		}
	}
	i := n - 1
	c, s50, hi, a := b[i].C, sma(b, i, 50), hiPrev(b, i, 20), atr(b, i, 20)
	p.Details = append(p.Details,
		fmt.Sprintf("آخر جلسة مكتملة: %s، إغلاق %.1f", b[i].Date, c),
		fmt.Sprintf("SMA50 = %.1f، أعلى سعر لآخر 20 جلسة سابقة = %.1f، ATR20 = %.1f", s50, hi, a))
	switch {
	case pos && pendingExit:
		p.State, p.StateText = "exit_pending", "مركز تجريبي مفتوح، وشرط الخروج تحقق (يُنفذ افتراضيا عند افتتاح الجلسة التالية)"
		p.Details = append(p.Details, fmt.Sprintf("دخول تجريبي %s عند %.1f، وقف %.1f", entryDate, entryPx, stop))
	case pos:
		p.State, p.StateText = "long", "مركز شراء تجريبي مفتوح"
		p.Details = append(p.Details, fmt.Sprintf("دخول تجريبي %s عند %.1f، وقف أولي %.1f", entryDate, entryPx, stop))
	case pending && partial:
		p.State, p.StateText = "expired", "تحققت إشارة دخول لكن الجلسة التالية بدأت بالفعل، فتُلغى الإشارة (انتهاء صلاحية الدخول الفائت)"
	case pending:
		p.State, p.StateText = "signal", "إشارة دخول تجريبية للجلسة التالية (تُلغى إذا بدأت الجلسة قبل التنفيذ)"
		p.Details = append(p.Details, fmt.Sprintf("وقف أولي مرجعي ≈ %.1f (إغلاق الإشارة − 2×ATR20)، التنفيذ الافتراضي عند افتتاح الجلسة التالية", c-2*a))
	default:
		p.State, p.StateText = "flat", "خارج السوق (لا مركز ولا إشارة)"
	}
	// gates for NEW entries
	if hs["gold"].Status != "ok" {
		p.Gates = append(p.Gates, "بيانات الذهب غير سليمة أو قديمة")
	}
	if hs["real"].Status != "ok" || hs["usd"].Status != "ok" {
		p.Gates = append(p.Gates, "بيانات الاقتصاد الكلي المطلوبة متأخرة أو غير متاحة")
	}
	p.Gates = append(p.Gates, "تقويم الأحداث الاقتصادية غير متاح في هذه النسخة، فتُعطل الدخول الجديد")
	p.Gates = append(p.Gates, "الأداة المالية الفعلية وتكاليفها غير محددة: وضع مراقبة فقط، وليست استراتيجية قابلة للتنفيذ")
	p.EntryOK = false
	p.TradeCount = len(trades)
	from := 0
	if len(trades) > 6 {
		from = len(trades) - 6
	}
	for k := len(trades) - 1; k >= from; k-- {
		t := trades[k]
		p.Trades = append(p.Trades, map[string]any{"entry": t.Entry, "exit": t.Exit, "entryPx": t.EntryPx, "exitPx": t.ExitPx, "ret": t.RetPct, "reason": t.Reason})
	}
	return p
}
