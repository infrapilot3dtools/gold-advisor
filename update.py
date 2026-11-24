import re, json, html, sys, urllib.request, datetime, os
URL="https://t.me/s/goldprice10000"
PREV=sys.argv[2] if len(sys.argv)>2 else ""
OUT=sys.argv[1]
def get(u):
    r=urllib.request.Request(u,headers={"User-Agent":"Mozilla/5.0 (compatible; gold-advisor)"})
    return urllib.request.urlopen(r,timeout=30).read().decode("utf-8","replace")
def digits(s):
    return s.translate(str.maketrans("٠١٢٣٤٥٦٧٨٩٬٫","0123456789,."))
def val(lines,*keys):
    for l in lines:
        for k in keys:
            if k in l:
                seg=l[l.find("←"):] if "←" in l else l
                m=re.search(r"[0-9][0-9,]*(?:\.[0-9]+)?",seg)
                if m: return float(m.group(0).replace(",",""))
    return None
page=get(URL); posts=[]
for blk in page.split('<div class="tgme_widget_message_wrap')[1:]:
    tm=re.search(r'<time datetime="([^"]+)"',blk); tx=re.search(r'<div class="tgme_widget_message_text[^>]*>(.*?)</div>',blk,re.S)
    if not tm or not tx: continue
    t=re.sub(r"<br\s*/?>","\n",tx.group(1),flags=re.I); t=digits(html.unescape(re.sub(r"<[^>]+>","",t)))
    if "عيار 21" not in t or "عيار 24" not in t: continue
    L=t.split("\n"); k24,k21,k18=val(L,"عيار 24"),val(L,"عيار 21"),val(L,"عيار 18")
    if not(k24 and k21 and k18) or k24<500 or abs(k21/k24-0.875)>0.01 or abs(k18/k24-0.75)>0.01: continue
    posts.append({"t":datetime.datetime.fromisoformat(tm.group(1)).astimezone(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
      "k24":k24,"k21":k21,"k18":k18,"pound":val(L,"جنيه ذهب"),"oz":val(L,"أونصة عالميا","أونصة عالميًا"),"bank":val(L,"البنك"),"par":val(L,"الموازي")})
if not posts: print("no valid posts"); sys.exit(1)
last=posts[-1]
hist=[]
if PREV:
    try: hist=json.loads(get(PREV)).get("history",[])
    except Exception as e: print("no prev",e)
have={h["t"] for h in hist}
for p in posts[-1:]:
    if p["t"] not in have: hist.append({"t":p["t"],"k21":p["k21"],"oz":p["oz"]})
hist=hist[-500:]
def parse(s): return datetime.datetime.strptime(s,"%Y-%m-%dT%H:%M:%SZ")
pts=[{"t":p["t"],"k21":p["k21"],"oz":p["oz"]} for p in posts]+hist
pts=sorted({p["t"]:p for p in pts}.values(),key=lambda p:p["t"])
now=parse(last["t"])
def chg(hours):
    base=None
    for p in pts:
        if (now-parse(p["t"])).total_seconds()>=hours*3600: base=p
    if base is None: return None
    return {"pct":round((last["k21"]/base["k21"]-1)*100,2),"hours":round((now-parse(base["t"])).total_seconds()/3600,1)}
d1,d7=chg(24),chg(24*7)
short=None
if len(pts)>1:
    f=pts[0]; hrs=(now-parse(f["t"])).total_seconds()/3600
    if hrs>=1: short={"pct":round((last["k21"]/f["k21"]-1)*100,2),"hours":round(hrs,1)}
ref=d1 or short
if ref is None: trend="unknown"
elif ref["pct"]>=0.4: trend="up"
elif ref["pct"]<=-0.4: trend="down"
else: trend="flat"
import math
span=(now-parse(pts[0]["t"])).total_seconds()/3600 if len(pts)>1 else 0
mom=ref["pct"] if ref else 0
conf=min(1.0,span/72)
dev=min(15,abs(50*math.tanh(mom/0.8))*(0.3+0.7*conf)) if ref else 0
up=round(50+(dev if mom>0 else -dev)); 
prob={"up":up,"down":100-up,"hours":24,"conf":"low" if span<72 else ("mid" if span<24*7 else "ok"),"span":round(span,1)}
out={"prob":prob,"generatedAt":datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ"),"price":last,"d1":d1,"d7":d7,"short":short,"trend":trend,"history":hist}
json.dump(out,open(OUT,"w"),ensure_ascii=False)
print(json.dumps({k:v for k,v in out.items() if k!="history"},ensure_ascii=False))
